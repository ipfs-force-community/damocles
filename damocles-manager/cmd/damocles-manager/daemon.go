package main

import (
	"context"
	"fmt"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/cmd/damocles-manager/internal"
	"github.com/ipfs-force-community/damocles/damocles-manager/dep"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/confmgr"
)

var daemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Commands for damocles-manager daemon",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		daemonInitCmd,
		daemonRunCmd,
	},
}

var daemonInitCmd = &cli.Command{
	Name:  "init",
	Usage: "Init the damocles-manager configuration files",
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

		procCfg := modules.DefaultProcessorConfig(true)
		if err := cfgmgr.SetDefault(cctx.Context, modules.ProcessorConfigKey, procCfg); err != nil {
			return fmt.Errorf("init ext prover config: %w", err)
		}

		log.Info("initialized")
		return nil
	},
}

var (
	daemonRunProxyFlag = &cli.StringFlag{
		Name:  "proxy",
		Usage: "Set a remote sector manager instance address as proxy",
	}

	daemonRunProxySectorIndexerOffFlag = &cli.BoolFlag{
		Name:  "proxy-sector-indexer-off",
		Usage: "Disable proxied sector-indexer",
	}
)

var daemonRunCmd = &cli.Command{
	Name:  "run",
	Usage: "Run the damocles-manager daemon",
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
		&cli.BoolFlag{
			Name:  "warmup",
			Value: true,
			Usage: "warmup for miner",
		},
		&cli.BoolFlag{
			Name:  "ext-prover",
			Value: false,
			Usage: "enable external prover",
		},
		&cli.BoolFlag{
			Name:  "worker-prover",
			Value: false,
			Usage: "enable worker prover",
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
		extProver, workerProver := cctx.Bool("ext-prover"), cctx.Bool("worker-prover")
		if extProver && workerProver {
			return fmt.Errorf("ext-prover and worker-prover are mutually exclusive")
		}

		var apiService *APIService
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
				dix.Override(new(dep.WinningPoStWarmUp), dep.WinningPoStWarmUp(cctx.Bool("warmup"))),
				dep.Miner(),
			),
			dep.Gateway(),
			dix.If(extProver, dep.ExtProver()),
			dix.If(workerProver, dep.WorkerProver()),
			dix.If(!workerProver, dep.DisableWorkerProver()),
			dep.Sealer(),
			dix.Override(new(*APIService), NewAPIService),
			dix.Populate(dep.InvokePopulate, &apiService),
		)
		if err != nil {
			return fmt.Errorf("construct api: %w", err)
		}

		return serveAPI(gctx, stopper, apiService, cctx.String("listen"))
	},
}
