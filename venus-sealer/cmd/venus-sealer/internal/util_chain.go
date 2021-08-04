package internal

import (
	"fmt"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/dep"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
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
		gctx, gcancel := NewSigContext(cctx.Context)
		defer gcancel()

		var api chain.API

		stopper, err := dix.New(
			gctx,
			DepsFromCLICtx(cctx),
			dix.Override(new(dep.GlobalContext), gctx),
			dep.Chain(&api),
		)
		if err != nil {
			return fmt.Errorf("construct sealer api: %w", err)
		}

		defer stopper(cctx.Context)

		head, err := api.ChainHead(gctx)
		if err != nil {
			return err
		}

		Log.Infof("ts: %s", head.Key())

		return nil
	},
}
