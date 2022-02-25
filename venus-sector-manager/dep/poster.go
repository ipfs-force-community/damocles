package dep

import (
	"context"

	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/poster"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

func PoSter() dix.Option {
	return dix.Options(
		dix.Override(StartPoSter, RunPoSter),
	)
}

func RunPoSter(
	gctx GlobalContext,
	lc fx.Lifecycle,
	scfg *modules.SafeConfig,
	verifier api.Verifier,
	prover api.Prover,
	indexer api.SectorIndexer,
	sectorTracker api.SectorTracker,
	capi chain.API,
	rapi api.RandomnessAPI,
	mapi messager.API,
) error {
	p, err := poster.NewPoSter(gctx, scfg, verifier, prover, indexer, sectorTracker, capi, rapi, mapi)
	if err != nil {
		return err
	}

	runCtx, runCancel := context.WithCancel(gctx)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go p.Run(runCtx)
			return nil
		},
		OnStop: func(context.Context) error {
			runCancel()
			return nil
		},
	})

	return nil
}
