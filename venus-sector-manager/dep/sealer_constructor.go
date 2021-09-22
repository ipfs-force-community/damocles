package dep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/commitmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/sectors"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/homedir"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
)

type (
	OnlineMetaStore             kvstore.KVStore
	OfflineMetaStore            kvstore.KVStore
	PersistedObjectStoreManager objstore.Manager
	SectorIndexMetaStore        kvstore.KVStore
)

func BuildLocalSectorManager(cfg *modules.Config, locker confmgr.RLocker, mapi api.MinerInfoAPI, numAlloc api.SectorNumberAllocator) (api.SectorManager, error) {
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

func ProvideConfig(gctx GlobalContext, lc fx.Lifecycle, cfgmgr confmgr.ConfigManager, locker confmgr.WLocker) (*modules.Config, error) {
	cfg := modules.DefaultConfig()
	if err := cfgmgr.Load(gctx, modules.ConfigKey, &cfg); err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Watch(gctx, modules.ConfigKey, &cfg, locker, func() interface{} {
				c := modules.DefaultConfig()
				return &c
			})
		},
	})

	return &cfg, nil
}

func ProvideSafeConfig(cfg *modules.Config, locker confmgr.RLocker) (*modules.SafeConfig, error) {
	return &modules.SafeConfig{
		Config: cfg,
		Locker: locker,
	}, nil
}

func BuildOnlineMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (OnlineMetaStore, error) {
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

func BuildOfflineMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (OfflineMetaStore, error) {
	dir := home.Sub("offline_meta")
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

func BuildSectorNumberAllocator(meta OnlineMetaStore) (api.SectorNumberAllocator, error) {
	store, err := kvstore.NewWrappedKVStore([]byte("sector-number"), meta)
	if err != nil {
		return nil, err
	}

	return sectors.NewNumerAllocator(store)
}

func BuildLocalSectorStateManager(online OnlineMetaStore, offline OfflineMetaStore) (api.SectorStateManager, error) {
	onlineStore, err := kvstore.NewWrappedKVStore([]byte("sector-states"), online)
	if err != nil {
		return nil, err
	}

	offlineStore, err := kvstore.NewWrappedKVStore([]byte("sector-states-offline"), offline)
	if err != nil {
		return nil, err
	}

	return sectors.NewStateManager(onlineStore, offlineStore)
}

func BuildMessagerClient(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.Config, locker confmgr.RLocker) (messager.API, error) {
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

func BuildChainClient(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.Config, locker confmgr.RLocker) (chain.API, error) {
	locker.Lock()
	api, token := scfg.Chain.Api, scfg.Chain.Token
	locker.Unlock()

	ccli, ccloser, err := chain.New(gctx, api, token)
	if err != nil {
		return chain.API{}, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			ccloser()
			return nil
		},
	})

	return ccli, nil
}

func BuildMinerInfoAPI(gctx GlobalContext, lc fx.Lifecycle, capi chain.API, scfg *modules.Config, locker confmgr.RLocker) (api.MinerInfoAPI, error) {
	mapi := chain.NewMinerInfoAPI(capi)

	locker.Lock()
	prefetch := scfg.SectorManager.PreFetch
	miners := scfg.SectorManager.Miners
	locker.Unlock()

	if prefetch && len(miners) > 0 {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				var wg sync.WaitGroup
				wg.Add(len(miners))

				for i := range miners {
					go func(mi int) {
						defer wg.Done()
						mid := miners[mi].ID

						mlog := log.With("miner", mid)
						_, err := mapi.Get(gctx, mid)
						if err == nil {
							mlog.Info("miner info pre-fetched")
						} else {
							mlog.Warnf("miner info pre-fetch failed: %v", err)
						}
					}(i)
				}

				wg.Wait()

				return nil
			},
		})
	}

	return mapi, nil
}

func BuildCommitmentManager(
	gctx GlobalContext,
	lc fx.Lifecycle,
	capi chain.API,
	mapi messager.API,
	rapi api.RandomnessAPI,
	stmgr api.SectorStateManager,
	scfg *modules.Config,
	rlock confmgr.RLocker,
	verif api.Verifier,
	prover api.Prover,
) (api.CommitmentManager, error) {
	mgr, err := commitmgr.NewCommitmentMgr(
		gctx,
		mapi,
		commitmgr.NewSealingAPIImpl(capi, rapi),
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
			mgr.Run(gctx)
			return nil
		},

		OnStop: func(ctx context.Context) error {
			mgr.Stop()
			return nil
		},
	})

	return mgr, nil
}

func BuildSectorIndexMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (SectorIndexMetaStore, error) {
	dir := home.Sub("sector-index")
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

func BuildPersistedFileStoreMgr(scfg *modules.Config, locker confmgr.RLocker) (PersistedObjectStoreManager, error) {
	locker.Lock()
	persistCfg := scfg.PersistedStore
	locker.Unlock()

	cfgs := make([]filestore.Config, 0)
	for _, include := range persistCfg.Includes {
		abs, err := filepath.Abs(include)
		if err != nil {
			return nil, fmt.Errorf("invalid include path %s: %w", include, err)
		}

		entries, err := os.ReadDir(abs)
		if err != nil {
			return nil, fmt.Errorf("read dir entries inside %s: %w", abs, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				log.Warnw("skipped non-dir entry", "include", abs, "sub", entry.Name())
				continue
			}

			cfgs = append(cfgs, filestore.DefaultConfig(filepath.Join(abs, entry.Name()), true))
		}
	}

	cfgs = append(cfgs, persistCfg.Stores...)

	return filestore.NewManager(cfgs)
}

func BuildSectorIndexer(storeMgr PersistedObjectStoreManager, kv SectorIndexMetaStore) (api.SectorIndexer, error) {
	return sectors.NewIndexer(storeMgr, kv)
}
