package dep

import (
	"context"

	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/poster"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
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
	verifier core.Verifier,
	prover core.Prover,
	sectorProving core.SectorProving,
	capi chain.API,
	rapi core.RandomnessAPI,
	mapi messager.API,
	minerAPI core.MinerAPI,
	senderSelect core.SenderSelector,
) error {
	p, err := poster.NewPoSter(scfg, capi, mapi, rapi, minerAPI, prover, verifier, sectorProving, senderSelect)
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
