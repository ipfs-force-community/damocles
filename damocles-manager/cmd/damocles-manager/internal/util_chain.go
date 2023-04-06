package internal

import (
	"fmt"
	"os"

	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/urfave/cli/v2"
)

var utilChainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		utilChainHeadCmd,
		utilChainPreCommitInfoCmd,
	},
}

var utilChainHeadCmd = &cli.Command{
	Name:  "head",
	Usage: "Print chain head",
	Action: func(cctx *cli.Context) error {
		api, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		head, err := api.Chain.ChainHead(gctx)
		if err != nil {
			return err
		}

		Log.Infof("ts %d: %s", head.Height(), head.Key())

		return nil
	},
}

var utilChainPreCommitInfoCmd = &cli.Command{
	Name:      "pci",
	Usage:     "Show on-chain pre-commit info for specified sector",
	ArgsUsage: "<miner actor id> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		maddr, err := ShouldAddress(cctx.Args().Get(0), true, true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := ShouldSectorNumber(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		pci, err := cli.Chain.StateSectorPreCommitInfo(gctx, maddr, sectorNum, types.EmptyTSK)
		if err != nil {
			return RPCCallError("StateSectorPreCommitInfo", err)
		}

		if err := OutputJSON(os.Stdout, pci); err != nil {
			return fmt.Errorf("output json: %w", err)
		}

		return nil
	},
}
