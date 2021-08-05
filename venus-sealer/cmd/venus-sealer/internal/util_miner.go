package internal

import (
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/urfave/cli/v2"
)

var utilMinerCmd = &cli.Command{
	Name: "miner",
	Subcommands: []*cli.Command{
		utilMinerInfoCmd,
	},
}

var utilMinerInfoCmd = &cli.Command{
	Name:      "info",
	ArgsUsage: "[miner address]",
	Action: func(cctx *cli.Context) error {
		addr := cctx.Args().First()
		if addr == "" {
			return ShowHelpf(cctx, "miner address is required")
		}

		var maddr address.Address
		var err error
		if actorID, perr := strconv.ParseUint(addr, 10, 64); perr == nil {
			maddr, err = address.NewIDAddress(actorID)
		} else {
			maddr, err = address.NewFromString(addr)
		}

		if err != nil {
			return fmt.Errorf("parse address from %s: %w", addr, err)
		}

		api, gctx, stop, err := extractChainAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		minfo, err := api.StateMinerInfo(gctx, maddr, types.EmptyTSK)
		if err != nil {
			return RPCCallError("StateMinerInfo", err)
		}

		fmt.Printf("Miner: %s\n", maddr)
		fmt.Printf("Owner: %s\n", minfo.Owner)
		fmt.Printf("Worker: %s\n", minfo.Worker)
		fmt.Printf("NewWorker: %s\n", minfo.NewWorker)

		fmt.Printf("Controls(%d):\n", len(minfo.ControlAddresses))
		for _, addr := range minfo.ControlAddresses {
			fmt.Printf("\t%s\n", addr)
		}

		fmt.Printf("WokerChangeEpoch: %d\n", minfo.WorkerChangeEpoch)
		if minfo.PeerId != nil {
			fmt.Printf("PeerID: %s\n", *minfo.PeerId)
		}

		fmt.Printf("Multiaddrs(%d):\n", len(minfo.Multiaddrs))
		for _, addr := range minfo.Multiaddrs {
			fmt.Printf("\t%s\n", addr)
		}

		fmt.Printf("SectorSize: %s\n", minfo.SectorSize)
		fmt.Printf("WindowPoStPartitionSectors: %d\n", minfo.WindowPoStPartitionSectors)

		return nil
	},
}
