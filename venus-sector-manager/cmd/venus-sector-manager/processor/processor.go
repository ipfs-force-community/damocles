package processor

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
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
	Name: prover.ExtProcessorNameWindostPoSt,
	Action: func(cctx *cli.Context) error {
		pid := os.Getpid()
		ppid := os.Getppid()
		plog := log.With("pid", pid, "ppid", ppid, "proc", prover.ExtProcessorNameWindostPoSt)

		plog.Info("ready")

		in := json.NewDecoder(os.Stdin)
		out := json.NewEncoder(os.Stdout)
		var outMu = &sync.Mutex{}

		for {
			var req prover.ExtRequest
			err := in.Decode(&req)
			if err != nil {
				plog.Warnf("decode incoming request: %s", err)
				continue
			}

			go func() {
				rlog := plog.With("id", req.ID, "data-size", len(req.Data))
				rlog.Debug("request arrived")

				resp := handleWdPoStReq(req)
				outMu.Lock()
				defer outMu.Unlock()

				start := time.Now()
				err := out.Encode(resp)
				rlog.Debugw("request done", "elapsed", time.Since(start).String())
				if err != nil {
					rlog.Warnf("encode response: %s", err)
				}

			}()
		}
	},
}

func handleWdPoStReq(req prover.ExtRequest) prover.ExtResponse {
	resp := prover.ExtResponse{
		ID: req.ID,
	}

	var data prover.WindowPoStData
	err := json.Unmarshal(req.Data, &data)
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

	resp.SetResult(prover.WindowPoStResult{
		Proof:   proof,
		Skipped: skipped,
	})

	return resp
}
