package dep

import (
	"context"

	"go.uber.org/fx"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/homedir"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/kvstore"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/messager"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/commitmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/sectors"
)

type (
	MetaStore kvstore.KVStore
)

func BuildLocalSectorManager(cfg *sealer.Config, locker confmgr.RLocker, mapi api.MinerInfoAPI, numAlloc api.SectorNumberAllocator) (api.SectorManager, error) {
	return sectors.NewManager(cfg, locker, mapi, numAlloc)
}

func BuildLocalConfigManager(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (confmgr.ConfigManager, error) {
	cfgmgr, err := confmgr.NewLocal(home.Dir())
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Run(gctx)
		},
		OnStop: func(ctx context.Context) error {
			return cfgmgr.Close(ctx)
		},
	})

	return cfgmgr, nil
}

func ProvideSealerConfig(gctx GlobalContext, lc fx.Lifecycle, cfgmgr confmgr.ConfigManager, locker confmgr.WLocker) (*sealer.Config, error) {
	cfg := sealer.DefaultConfig()
	if err := cfgmgr.Load(gctx, sealer.ConfigKey, &cfg); err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Watch(gctx, sealer.ConfigKey, &cfg, locker, func() interface{} {
				c := sealer.DefaultConfig()
				return &c
			})
		},
	})

	return &cfg, nil
}

func BuildMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (MetaStore, error) {
	dir := home.Sub("meta")
	store, err := kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return store.Run(gctx)
		},

		OnStop: func(ctx context.Context) error {
			return store.Close(ctx)
		},
	})

	return store, nil
}

func BuildSectorNumberAllocator(meta MetaStore) (api.SectorNumberAllocator, error) {
	store, err := kvstore.NewWrappedKVStore([]byte("sector-number"), meta)
	if err != nil {
		return nil, err
	}

	return sectors.NewNumerAllocator(store)
}

func BuildLocalSectorStateManager(meta MetaStore) (api.SectorStateManager, error) {
	store, err := kvstore.NewWrappedKVStore([]byte("sector-states"), meta)
	if err != nil {
		return nil, err
	}

	return sectors.NewStateManager(store)
}

func BuildMessagerClient(gctx GlobalContext, lc fx.Lifecycle, scfg *sealer.Config, locker confmgr.RLocker) (messager.API, error) {
	locker.Lock()
	api, token := scfg.Messager.Api, scfg.Messager.Token
	locker.Unlock()

	mcli, mcloser, err := messager.New(gctx, api, token)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			mcloser()
			return nil
		},
	})

	return mcli, nil
}

func BuildChainClient(gctx GlobalContext, lc fx.Lifecycle, scfg *sealer.Config, locker confmgr.RLocker) (chain.API, error) {
	locker.Lock()
	api, token := scfg.Chain.Api, scfg.Chain.Token
	locker.Unlock()

	mcli, mcloser, err := chain.New(gctx, api, token)
	if err != nil {
		return chain.API{}, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			mcloser()
			return nil
		},
	})

	return mcli, nil
}

func BuildCommitmentManager(
	gctx GlobalContext,
	lc fx.Lifecycle,
	capi chain.API,
	mapi messager.API,
	stmgr api.SectorStateManager,
	scfg *sealer.Config,
	rlock confmgr.RLocker,
	verif api.Verifier,
	prover api.Prover,
) (api.CommitmentManager, error) {
	mgr, err := commitmgr.NewCommitmentMgr(
		gctx,
		mapi,
		commitmgr.NewSealingAPIImpl(capi),
		stmgr,
		scfg,
		rlock,
		verif,
		prover,
	)

	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			mgr.Run()
			return nil
		},

		OnStop: func(ctx context.Context) error {
			mgr.Stop()
			return nil
		},
	})

	return mgr, nil
}
