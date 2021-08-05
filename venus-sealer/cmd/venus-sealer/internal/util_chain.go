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
		api, gctx, stop, err := extractChainAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		head, err := api.ChainHead(gctx)
		if err != nil {
			return err
		}

		Log.Infof("ts: %s", head.Key())

		return nil
	},
}
