package main

import (
	"context"
	"fmt"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/internal"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/dep"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
)

var daemonCmd = &cli.Command{
	Name:  "daemon",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		daemonInitCmd,
		daemonRunCmd,
	},
}

var daemonInitCmd = &cli.Command{
	Name: "init",
	Action: func(cctx *cli.Context) error {
		home, err := internal.HomeFromCLICtx(cctx)
		if err != nil {
			return err
		}

		cfgmgr, err := confmgr.NewLocal(home.Dir())
		if err != nil {
			return fmt.Errorf("construct config manager: %w", err)
		}

		cfg := modules.ExampleConfig()
		if err := cfgmgr.SetDefault(cctx.Context, modules.ConfigKey, cfg); err != nil {
			return fmt.Errorf("init sealer config: %w", err)
		}

		log.Info("initialized")
		return nil
	},
}

var daemonRunCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		internal.SealerListenFlag,
		&cli.BoolFlag{
			Name:  "poster",
			Value: false,
			Usage: "enable poster module",
		},
		&cli.BoolFlag{
			Name:  "miner",
			Value: false,
			Usage: "enable miner module",
		},
	},
	Action: func(cctx *cli.Context) error {
		gctx, gcancel := internal.NewSigContext(context.Background())
		defer gcancel()

		var node api.SealerAPI
		stopper, err := dix.New(
			gctx,
			internal.DepsFromCLICtx(cctx),
			dix.Override(new(dep.GlobalContext), gctx),
			dep.Product(),
			dix.If(
				cctx.Bool("poster"),
				dep.PoSter(),
			),
			dix.If(
				cctx.Bool("miner"),
				dep.Miner(),
			),
			dep.Sealer(&node),
		)
		if err != nil {
			return fmt.Errorf("construct sealer api: %w", err)
		}

		return serveSealerAPI(gctx, stopper, node, cctx.String("listen"))
	},
}
