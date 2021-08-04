package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/dtynn/dix"
	"github.com/dtynn/venus-cluster/venus-sealer/dep"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/homedir"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var daemonCmd = &cli.Command{
	Name: "daemon",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "home",
			Value: "~/.venus-sealer",
		},
	},
	Subcommands: []*cli.Command{
		daemonInitCmd,
		daemonRunCmd,
	},
}

var daemonInitCmd = &cli.Command{
	Name: "init",
	Action: func(cctx *cli.Context) error {
		home, err := homedir.Open(cctx.String("home"))
		if err != nil {
			return fmt.Errorf("open home: %w", err)
		}

		if err := home.Init(); err != nil {
			return fmt.Errorf("init home: %w", err)
		}

		cfgmgr, err := confmgr.NewLocal(home.Dir())
		if err != nil {
			return fmt.Errorf("construct config manager: %w", err)
		}

		cfg := sealer.DefaultConfig()
		if err := cfgmgr.SetDefault(cctx.Context, sealer.ConfigKey, cfg); err != nil {
			return fmt.Errorf("init sealer config: %w", err)
		}

		log.Info("initialized")
		return nil
	},
}

var daemonRunCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: ":1789",
		},
	},
	Action: func(cctx *cli.Context) error {
		home, err := homedir.Open(cctx.String("home"))
		if err != nil {
			return fmt.Errorf("open home: %s", err)
		}

		gctx, gcancel := newSigContext(context.Background())
		defer gcancel()

		var node api.SealerAPI
		stopper, err := dix.New(
			cctx.Context,
			dix.Override(new(dep.GlobalContext), gctx),
			dix.Override(new(*homedir.Home), home),
			dep.Product(),
			dep.Sealer(&node),
		)
		if err != nil {
			return fmt.Errorf("construct sealer api: %w", err)
		}

		return serveSealerAPI(gctx, stopper, node, cctx.String("listen"))
	},
}
