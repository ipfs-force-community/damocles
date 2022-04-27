package processor

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover/ext"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("processor-cmd")

var ProcessorCmd = &cli.Command{
	Name:  "processor",
	Usage: "ext processors for prover",
	Subcommands: []*cli.Command{
		processorWdPostCmd,
	},
}

var processorWdPostCmd = &cli.Command{
	Name: ext.ProcessorNameWindostPoSt,
	Action: func(cctx *cli.Context) error {
		pid := os.Getpid()
		ppid := os.Getppid()
		plog := log.With("pid", pid, "ppid", ppid, "proc", ext.ProcessorNameWindostPoSt)

		plog.Info("ready")

		in := bufio.NewScanner(os.Stdin)
		out := bufio.NewWriter(os.Stdout)
		var outMu = &sync.Mutex{}

		for {
			if !in.Scan() {
				break
			}

			var req ext.Request
			b := in.Bytes()
			err := json.Unmarshal(b, &req)
			if err != nil {
				plog.Warnf("decode incoming request: %s", err)
				continue
			}

			go func() {
				rlog := plog.With("id", req.ID, "req-bytes", len(b), "data-bytes", len(req.Data))
				rlog.Debug("request arrived")

				resp := handleWdPoStReq(req)
				outMu.Lock()
				defer outMu.Unlock()

				start := time.Now()
				n, err := ext.WriteData(out, resp)
				if err != nil {
					rlog.Warnf("encode response: %s", err)
				}

				rlog.Debugw("request done", "elapsed", time.Since(start).String(), "res-bytes", len(resp.Result), "resp-bytes", n)

			}()
		}

		if err := in.Err(); err != nil {
			plog.Warnf("stdin broken: %s", err)
		}

		return nil
	},
}

func handleWdPoStReq(req ext.Request) ext.Response {
	resp := ext.Response{
		ID: req.ID,
	}

	var data ext.WindowPoStData
	err := req.DecodeInto(&data)
	if err != nil {
		errMsg := err.Error()
		resp.ErrMsg = &errMsg
		return resp
	}

	proof, skipped, err := prover.Prover.GenerateWindowPoSt(context.Background(), data.Miner, data.Sectors, data.Randomness)
	if err != nil {
		errMsg := err.Error()
		resp.ErrMsg = &errMsg
		return resp
	}

	resp.SetResult(ext.WindowPoStResult{
		Proof:   proof,
		Skipped: skipped,
	})

	return resp
}
