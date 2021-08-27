package internal

import (
	"github.com/urfave/cli/v2"
)

var utilChainCmd = &cli.Command{
	Name: "chain",
	Subcommands: []*cli.Command{
		utilChainHeadCmd,
	},
}

var utilChainHeadCmd = &cli.Command{
	Name: "head",
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

		Log.Infof("ts: %s", head.Key())

		return nil
	},
}
