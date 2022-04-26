package main

import (
	"context"
	"fmt"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/cmd/venus-sector-manager/internal"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
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

		cfg := modules.DefaultConfig(true)
		if err := cfgmgr.SetDefault(cctx.Context, modules.ConfigKey, cfg); err != nil {
			return fmt.Errorf("init sealer config: %w", err)
		}

		log.Info("initialized")
		return nil
	},
}

var (
	daemonRunProxyFlag = &cli.StringFlag{
		Name:  "proxy",
		Usage: "set a remote sector manager instance address as proxy",
	}

	daemonRunProxySectorIndexerOffFlag = &cli.BoolFlag{
		Name:  "proxy-sector-indexer-off",
		Usage: "disable proxied sector-indexer",
	}
)

var daemonRunCmd = &cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		internal.SealerListenFlag,
		internal.ConfDirFlag,
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
		daemonRunProxyFlag,
		daemonRunProxySectorIndexerOffFlag,
	},
	Action: func(cctx *cli.Context) error {
		gctx, gcancel := internal.NewSigContext(context.Background())
		defer gcancel()

		proxy := cctx.String(daemonRunProxyFlag.Name)
		proxyOpt := dep.ProxyOptions{
			EnableSectorIndexer: !cctx.Bool(daemonRunProxySectorIndexerOffFlag.Name),
		}

		var node core.SealerAPI
		stopper, err := dix.New(
			gctx,
			dep.Product(),
			internal.DepsFromCLICtx(cctx),
			dix.Override(new(dep.GlobalContext), gctx),
			dix.If(proxy != "", dep.Proxy(proxy, proxyOpt)),
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
