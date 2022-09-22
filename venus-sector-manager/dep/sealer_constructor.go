package dep

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"
	vapi "github.com/filecoin-project/venus/venus-shared/api"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/commitmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/dealmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/mock"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/sectors"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/worker"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/homedir"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/piecestore"
)

type (
	OnlineMetaStore             kvstore.KVStore
	OfflineMetaStore            kvstore.KVStore
	PersistedObjectStoreManager objstore.Manager
	SectorIndexMetaStore        kvstore.KVStore
	SnapUpMetaStore             kvstore.KVStore
	ListenAddress               string
	ProxyAddress                string
	WorkerMetaStore             kvstore.KVStore
	ConfDirPath                 string
	CommonMetaStore             kvstore.KVStore
)

func BuildLocalSectorManager(scfg *modules.SafeConfig, mapi core.MinerInfoAPI, numAlloc core.SectorNumberAllocator) (core.SectorManager, error) {
	return sectors.NewManager(scfg, mapi, numAlloc)
}

func BuildConfDirPath(home *homedir.Home) ConfDirPath {
	return ConfDirPath(home.Dir())
}

func BuildLocalConfigManager(gctx GlobalContext, lc fx.Lifecycle, confDir ConfDirPath) (confmgr.ConfigManager, error) {
	cfgmgr, err := confmgr.NewLocal(string(confDir))
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
	cfg := modules.DefaultConfig(false)
	if err := cfgmgr.Load(gctx, modules.ConfigKey, &cfg); err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}
	encode := toml.NewEncoder(&buf)
	encode.Indent = ""
	err := encode.Encode(cfg)
	if err != nil {
		return nil, err
	}

	log.Infof("Sector-manager initial cfg: \n%s\n", buf.String())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Watch(gctx, modules.ConfigKey, &cfg, locker, func() interface{} {
				c := modules.DefaultConfig(false)
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

func BuildCommonMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (CommonMetaStore, error) {
	var store kvstore.KVStore
	var err error
	sub := "common"
	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func BuildOnlineMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (OnlineMetaStore, error) {
	var store kvstore.KVStore
	var err error

	sub := "meta"
	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func BuildOfflineMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (OfflineMetaStore, error) {
	var store kvstore.KVStore
	var err error
	sub := "offline_meta"

	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func BuildSectorNumberAllocator(meta OnlineMetaStore) (core.SectorNumberAllocator, error) {
	store, err := kvstore.NewWrappedKVStore([]byte("sector-number"), meta)
	if err != nil {
		return nil, err
	}

	return sectors.NewNumerAllocator(store)
}

func BuildLocalSectorStateManager(online OnlineMetaStore, offline OfflineMetaStore) (core.SectorStateManager, error) {
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
	api, token := extractAPIInfo(scfg.Common.API.Messager, scfg.Common.API.Token)
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

// used for cli commands
func MaybeSealerCliClient(gctx GlobalContext, lc fx.Lifecycle, listen ListenAddress) core.SealerCliClient {
	cli, err := buildSealerCliClient(gctx, lc, string(listen), false)
	if err != nil {
		cli = core.UnavailableSealerCliClient
	}

	return cli
}

// used for proxy
func BuildSealerProxyClient(gctx GlobalContext, lc fx.Lifecycle, proxy ProxyAddress) (core.SealerCliClient, error) {
	return buildSealerCliClient(gctx, lc, string(proxy), true)
}

func buildSealerCliClient(gctx GlobalContext, lc fx.Lifecycle, serverAddr string, useHTTP bool) (core.SealerCliClient, error) {
	var scli core.SealerCliClient

	addr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		return scli, err
	}

	ip := addr.IP
	if ip == nil || ip.Equal(net.IPv4zero) {
		ip = net.IPv4(127, 0, 0, 1)
	}

	maddr := fmt.Sprintf("/ip4/%s/tcp/%d", ip, addr.Port)
	if useHTTP {
		maddr += "/http"
	}

	ainfo := vapi.NewAPIInfo(maddr, "")
	apiAddr, err := ainfo.DialArgs(vapi.VerString(core.MajorVersion))
	if err != nil {
		return scli, err
	}

	closer, err := jsonrpc.NewMergeClient(gctx, apiAddr, "Venus", []interface{}{&scli}, ainfo.AuthHeader(), jsonrpc.WithRetry(true))
	if err != nil {
		return scli, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			closer()
			return nil
		},
	})

	return scli, nil
}

func BuildChainClient(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.Config, locker confmgr.RLocker) (chain.API, error) {
	locker.Lock()
	api, token := extractAPIInfo(scfg.Common.API.Chain, scfg.Common.API.Token)
	locker.Unlock()

	ccli, ccloser, err := chain.New(gctx, api, token)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			ccloser()
			return nil
		},
	})

	return ccli, nil
}

func BuildMinerInfoAPI(gctx GlobalContext, lc fx.Lifecycle, capi chain.API, scfg *modules.Config, locker confmgr.RLocker) (core.MinerInfoAPI, error) {
	mapi := chain.NewMinerInfoAPI(capi)

	locker.Lock()
	miners := scfg.Miners
	locker.Unlock()

	if len(miners) > 0 {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				var wg sync.WaitGroup
				wg.Add(len(miners))

				for i := range miners {
					go func(mi int) {
						defer wg.Done()
						mid := miners[mi].Actor

						mlog := log.With("miner", mid)
						info, err := mapi.Get(gctx, mid)
						if err == nil {
							mlog.Infof("miner info pre-fetched: %#v", info)
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
	rapi core.RandomnessAPI,
	stmgr core.SectorStateManager,
	minfoAPI core.MinerInfoAPI,
	scfg *modules.SafeConfig,
	verif core.Verifier,
	prover core.Prover,
) (core.CommitmentManager, error) {
	mgr, err := commitmgr.NewCommitmentMgr(
		gctx,
		mapi,
		commitmgr.NewSealingAPIImpl(capi, rapi),
		minfoAPI,
		stmgr,
		scfg,
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

func BuildSectorIndexMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (SectorIndexMetaStore, error) {
	var store kvstore.KVStore
	var err error
	sub := "sector-index"

	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func BuildSnapUpMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (SnapUpMetaStore, error) {
	var store kvstore.KVStore
	var err error
	sub := "snapup"

	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func openObjStore(cfg objstore.Config, pluginPath string) (objstore.Store, error) {
	if cfg.Name == "" {
		cfg.Name = cfg.Path
	}

	if pluginPath == "" {
		st, err := filestore.Open(cfg, false)
		if err != nil {
			return nil, fmt.Errorf("open filestore: %w", err)
		}

		log.Infow("embed store constructed", "type", st.Type(), "ver", st.Version(), "instance", st.Instance(context.Background()))
		return st, nil
	}

	absPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("get abs path of objstore plugin %s: %w", pluginPath, err)
	}

	constructor, err := objstore.LoadConstructor(absPath)
	if err != nil {
		return nil, fmt.Errorf("load objstore constructor from %s: %w", absPath, err)
	}

	st, err := constructor(cfg)
	if err != nil {
		return nil, fmt.Errorf("construct objstore use plugin %s: %w", absPath, err)
	}

	log.Infow("store constructed", "type", st.Type(), "ver", st.Version(), "instance", st.Instance(context.Background()))

	return st, nil
}

func BuildPersistedFileStoreMgr(scfg *modules.Config, locker confmgr.RLocker, globalStore CommonMetaStore) (PersistedObjectStoreManager, error) {
	locker.Lock()
	persistCfg := scfg.Common.PersistStores
	locker.Unlock()

	stores := make([]objstore.Store, 0, len(persistCfg))
	policy := map[string]objstore.StoreSelectPolicy{}
	for pi := range persistCfg {
		st, err := openObjStore(persistCfg[pi].Config, persistCfg[pi].Plugin)
		if err != nil {
			return nil, fmt.Errorf("construct #%d persist store: %w", pi, err)
		}

		stores = append(stores, st)
		policy[st.Instance(context.Background())] = persistCfg[pi].StoreSelectPolicy
	}

	wrapped, err := kvstore.NewWrappedKVStore([]byte("objstore"), globalStore)
	if err != nil {
		return nil, fmt.Errorf("construct wrapped kv store for objstore: %w", err)
	}

	return objstore.NewStoreManager(stores, policy, wrapped)
}

func BuildSectorIndexer(storeMgr PersistedObjectStoreManager, kv SectorIndexMetaStore) (core.SectorIndexer, error) {
	upgrade, err := kvstore.NewWrappedKVStore([]byte("sector-upgrade"), kv)
	if err != nil {
		return nil, fmt.Errorf("wrap kvstore for sector-upgrade: %w", err)
	}

	return sectors.NewIndexer(storeMgr, kv, upgrade)
}

func BuildSectorTracker(indexer core.SectorIndexer) (core.SectorTracker, error) {
	return sectors.NewTracker(indexer)
}

type MarketAPIRelatedComponets struct {
	fx.Out

	DealManager core.DealManager
	MarketAPI   market.API
}

func BuildMarketAPI(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, infoAPI core.MinerInfoAPI) (market.API, error) {
	scfg.Lock()
	api, token := extractAPIInfo(scfg.Common.API.Market, scfg.Common.API.Token)
	defer scfg.Unlock()

	if api == "" {
		return nil, nil
	}

	mapi, mcloser, err := market.New(gctx, api, token)
	if err != nil {
		return nil, fmt.Errorf("construct market api: %w", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			mcloser()
			return nil
		},
	})

	return mapi, nil
}

func BuildMarketAPIRelated(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, infoAPI core.MinerInfoAPI, capi chain.API) (MarketAPIRelatedComponets, error) {
	mapi, err := BuildMarketAPI(gctx, lc, scfg, infoAPI)
	if err != nil {
		return MarketAPIRelatedComponets{}, fmt.Errorf("build market api: %w", err)
	}

	if mapi == nil {
		log.Warn("deal manager based on market api is disabled, use mocked")
		return MarketAPIRelatedComponets{
			DealManager: mock.NewDealManager(),
			MarketAPI:   nil,
		}, nil
	}

	scfg.Lock()
	pieceStoreCfg := scfg.Common.PieceStores
	scfg.Unlock()

	stores := make([]objstore.Store, 0, len(pieceStoreCfg))
	for pi := range pieceStoreCfg {
		pcfg := pieceStoreCfg[pi]
		cfg := objstore.Config{
			Name:     pcfg.Name,
			Path:     pcfg.Path,
			Meta:     pcfg.Meta,
			ReadOnly: true,
		}
		st, err := openObjStore(cfg, pcfg.Plugin)
		if err != nil {
			return MarketAPIRelatedComponets{}, fmt.Errorf("construct #%d piece store: %w", pi, err)
		}

		stores = append(stores, st)
	}

	proxy := piecestore.NewProxy(stores, mapi)
	http.DefaultServeMux.Handle(HTTPEndpointPiecestore, http.StripPrefix(HTTPEndpointPiecestore, proxy))
	log.Info("piecestore proxy has been registered into default mux")

	return MarketAPIRelatedComponets{
		DealManager: dealmgr.New(mapi, infoAPI, scfg, capi),
		MarketAPI:   mapi,
	}, nil
}

func BuildChainEventBus(
	gctx GlobalContext,
	lc fx.Lifecycle,
	capi chain.API,
	scfg *modules.SafeConfig,
) (*chain.EventBus, error) {
	scfg.Lock()
	interval := scfg.Common.API.ChainEventInterval
	scfg.Unlock()

	bus, err := chain.NewEventBus(gctx, capi, interval.Std())
	if err != nil {
		return nil, fmt.Errorf("construct chain eventbus: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go bus.Run()
			return nil
		},

		OnStop: func(ctx context.Context) error {
			bus.Stop()
			return nil
		},
	})

	return bus, nil
}

func BuildSnapUpManager(
	gctx GlobalContext,
	lc fx.Lifecycle,
	scfg *modules.SafeConfig,
	tracker core.SectorTracker,
	indexer core.SectorIndexer,
	chainAPI chain.API,
	eventbus *chain.EventBus,
	messagerAPI messager.API,
	minerInfoAPI core.MinerInfoAPI,
	stateMgr core.SectorStateManager,
	store SnapUpMetaStore,
) (core.SnapUpSectorManager, error) {
	mgr, err := sectors.NewSnapUpMgr(gctx, tracker, indexer, chainAPI, eventbus, messagerAPI, minerInfoAPI, stateMgr, scfg, store)
	if err != nil {
		return nil, fmt.Errorf("construct snapup manager: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return mgr.Start()
		},

		OnStop: func(ctx context.Context) error {
			mgr.Stop()
			return nil
		},
	})

	return mgr, nil
}

func BuildWorkerMetaStore(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home, scfg *modules.SafeConfig) (WorkerMetaStore, error) {
	var store kvstore.KVStore
	var err error
	sub := "worker"

	if scfg.Common.MongoKVStore.Enable {
		mongoCfg := scfg.Common.MongoKVStore
		store, err = kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName, sub)
		if err != nil {
			return nil, err
		}
	} else {
		dir := home.Sub(sub)
		store, err = kvstore.OpenBadger(kvstore.DefaultBadgerOption(dir))
		if err != nil {
			return nil, err
		}
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

func BuildWorkerManager(meta WorkerMetaStore) (core.WorkerManager, error) {
	return worker.NewManager(meta)
}

func BuildProxiedSectorIndex(client core.SealerCliClient, storeMgr PersistedObjectStoreManager) (core.SectorIndexer, error) {
	log.Debug("build proxied sector indexer")
	return sectors.NewProxiedIndexer(client, storeMgr)
}
