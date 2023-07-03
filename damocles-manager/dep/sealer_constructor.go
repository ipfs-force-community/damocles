package dep

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/BurntSushi/toml"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"
	vapi "github.com/filecoin-project/venus/venus-shared/api"
	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/commitmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/dealmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/mock"
	proverworker "github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover/worker"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/sectors"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/worker"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/confmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/homedir"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/market"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore/filestore"
	objstoreplugin "github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore/plugin"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/piecestore"
)

type (
	UnderlyingDB                kvstore.DB
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
	WorkerProverStore           kvstore.KVStore
)

func BuildLocalSectorManager(scfg *modules.SafeConfig, mapi core.MinerAPI, numAlloc core.SectorNumberAllocator) (core.SectorManager, error) {
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

func ProvidePlugins(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig) (*managerplugin.LoadedPlugins, error) {
	pluginsConfig := scfg.MustCommonConfig().Plugins
	if pluginsConfig == nil {
		pluginsConfig = modules.DefaultPluginConfig()
	}
	plugins, err := managerplugin.Load(pluginsConfig.Dir)
	if err != nil {
		return nil, err
	}
	plugins.Init(gctx, func(p *managerplugin.Plugin, err error) {
		log.Warnf("call Plugin OnInit failure '%s/%s', err: %s", p.Kind, p.Name, err)
	})
	_ = plugins.ForeachAllKind(func(p *managerplugin.Plugin) error {
		log.Infof("loaded plugin '%s/%s', build time: '%s'.", p.Kind, p.Name, p.BuildTime)
		return nil
	})
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return nil
		},

		OnStop: func(ctx context.Context) error {
			plugins.Shutdown(ctx, func(p *managerplugin.Plugin, err error) {
				log.Warnf("call OnShutdown for failure '%s/%s', err: %w", p.Kind, p.Name, err)
			})
			return nil
		},
	})
	return plugins, nil
}

func BuildUnderlyingDB(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, home *homedir.Home, loadedPlugins *managerplugin.LoadedPlugins) (UnderlyingDB, error) {
	commonCfg := scfg.MustCommonConfig()

	var dbCfg modules.DBConfig
	if commonCfg.MongoKVStore != nil && commonCfg.MongoKVStore.Enable { // For compatibility with v0.5
		dbCfg.Driver = "mongo"
		dbCfg.Mongo = commonCfg.MongoKVStore
	} else {
		if commonCfg.DB == nil {
			commonCfg.DB = modules.DefaultDBConfig()
		}
		dbCfg = *commonCfg.DB
	}
	return BuildKVStoreDB(gctx, lc, dbCfg, home, loadedPlugins)
}

func BuildKVStoreDB(gctx GlobalContext, lc fx.Lifecycle, cfg modules.DBConfig, home *homedir.Home, loadedPlugins *managerplugin.LoadedPlugins) (UnderlyingDB, error) {
	switch cfg.Driver {
	case "badger", "Badger":
		return BuildKVStoreBadgerDB(lc, cfg.Badger, home)
	case "mongo", "Mongo":
		return BuildKVStoreMongoDB(gctx, lc, cfg.Mongo)
	case "plugin", "Plugin":
		return BuildPluginDB(lc, cfg.Plugin, loadedPlugins)
	default:
		return nil, fmt.Errorf("unsupported db driver '%s'", cfg.Driver)
	}
}

func BuildKVStoreBadgerDB(lc fx.Lifecycle, badgerCfg *modules.KVStoreBadgerDBConfig, home *homedir.Home) (UnderlyingDB, error) {
	if badgerCfg == nil {
		*badgerCfg = *modules.DefaultDBConfig().Badger
	}

	baseDir := badgerCfg.BaseDir
	if baseDir == "" {
		baseDir = home.Dir()
	}
	db := kvstore.OpenBadger(baseDir)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.Run(ctx)
		},

		OnStop: func(ctx context.Context) error {
			return db.Close(ctx)
		},
	})

	return db, nil
}

func BuildKVStoreMongoDB(gctx GlobalContext, lc fx.Lifecycle, mongoCfg *modules.KVStoreMongoDBConfig) (UnderlyingDB, error) {
	if mongoCfg == nil {
		return nil, fmt.Errorf("invalid mongodb config")
	}

	db, err := kvstore.OpenMongo(gctx, mongoCfg.DSN, mongoCfg.DatabaseName)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.Run(ctx)
		},

		OnStop: func(ctx context.Context) error {
			return db.Close(ctx)
		},
	})

	return db, err
}

func BuildPluginDB(lc fx.Lifecycle, pluginDBCfg *modules.KVStorePluginDBConfig, loadedPlugins *managerplugin.LoadedPlugins) (UnderlyingDB, error) {
	if pluginDBCfg == nil {
		return nil, fmt.Errorf("invalid plugin db config")
	}
	db, err := kvstore.OpenPluginDB(pluginDBCfg.PluginName, pluginDBCfg.Meta, loadedPlugins)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.Run(ctx)
		},

		OnStop: func(ctx context.Context) error {
			return db.Close(ctx)
		},
	})
	return db, err
}

func BuildCommonMetaStore(gctx GlobalContext, db UnderlyingDB) (CommonMetaStore, error) {
	return db.OpenCollection(gctx, "common")
}

func BuildOnlineMetaStore(gctx GlobalContext, db UnderlyingDB) (OnlineMetaStore, error) {
	return db.OpenCollection(gctx, "meta")
}

func BuildOfflineMetaStore(gctx GlobalContext, db UnderlyingDB) (OfflineMetaStore, error) {
	return db.OpenCollection(gctx, "offline_meta")
}

func BuildSectorNumberAllocator(meta OnlineMetaStore) (core.SectorNumberAllocator, error) {
	store, err := kvstore.NewWrappedKVStore([]byte("sector-number"), meta)
	if err != nil {
		return nil, err
	}

	return sectors.NewNumberAllocator(store)
}

func BuildLocalSectorStateManager(online OnlineMetaStore, offline OfflineMetaStore, loadedPlugins *managerplugin.LoadedPlugins) (core.SectorStateManager, error) {
	onlineStore, err := kvstore.NewWrappedKVStore([]byte("sector-states"), online)
	if err != nil {
		return nil, err
	}

	offlineStore, err := kvstore.NewWrappedKVStore([]byte("sector-states-offline"), offline)
	if err != nil {
		return nil, err
	}

	return sectors.NewStateManager(onlineStore, offlineStore, loadedPlugins)
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

func MaybeAPIClient(gctx GlobalContext, lc fx.Lifecycle, listen ListenAddress) *core.APIClient {
	var client core.APIClient
	err := buildDamoclesAPIClient(gctx, lc, core.APINamespace, &client, string(listen), false)
	if err != nil {
		log.Errorf("failed to build api client. err: %s", err)
		client = core.UnavailableAPIClient
	}

	return &client
}

// used for proxy
func BuildAPIProxyClient(gctx GlobalContext, lc fx.Lifecycle, proxy ProxyAddress) (*core.APIClient, error) {
	var proxyClient core.APIClient
	err := buildDamoclesAPIClient(gctx, lc, core.APINamespace, &proxyClient, string(proxy), true)
	return &proxyClient, err
}

func BuildSealerCliAPIClient(client *core.APIClient) *core.SealerCliAPIClient {
	return &client.SealerCliAPIClient
}

func buildDamoclesAPIClient(gctx GlobalContext, lc fx.Lifecycle, namespace string, out interface{}, serverAddr string, useHTTP bool) error {
	addr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		return err
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
		return err
	}

	closer, err := jsonrpc.NewMergeClient(gctx, apiAddr, namespace, []interface{}{out}, ainfo.AuthHeader(), jsonrpc.WithRetry(true))
	if err != nil {
		return err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			closer()
			return nil
		},
	})

	return nil
}

func BuildChainClient(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.Config, locker confmgr.RLocker) (chain.API, error) {
	locker.Lock()
	api, token := extractAPIInfo(scfg.Common.API.Chain, scfg.Common.API.Token)
	locker.Unlock()

	ccli, ccloser, err := chain.New(gctx, api, token)
	if err != nil {
		return nil, err
	}

	err = policy.SetupNetwork(gctx, ccli)
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

func BuildMinerAPI(gctx GlobalContext, lc fx.Lifecycle, capi chain.API, scfg *modules.SafeConfig) (core.MinerAPI, error) {
	mapi := chain.NewMinerAPI(capi, scfg)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			mapi.PrefetchCache(ctx)
			return nil
		},
	})
	return mapi, nil
}

func BuildCommitmentManager(
	gctx GlobalContext,
	lc fx.Lifecycle,
	capi chain.API,
	mapi messager.API,
	rapi core.RandomnessAPI,
	stmgr core.SectorStateManager,
	minerAPI core.MinerAPI,
	scfg *modules.SafeConfig,
	verif core.Verifier,
	prover core.Prover,
) (core.CommitmentManager, error) {
	mgr, err := commitmgr.NewCommitmentMgr(
		gctx,
		mapi,
		commitmgr.NewSealingAPIImpl(capi, rapi),
		minerAPI,
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

func BuildSectorIndexMetaStore(gctx GlobalContext, db UnderlyingDB) (SectorIndexMetaStore, error) {
	return db.OpenCollection(gctx, "sector-index")
}

func BuildSnapUpMetaStore(gctx GlobalContext, db UnderlyingDB) (SnapUpMetaStore, error) {
	return db.OpenCollection(gctx, "snapup")
}

func openObjStore(cfg objstore.Config, pluginName string, loadedPlugins *managerplugin.LoadedPlugins) (st objstore.Store, err error) {
	if cfg.Name == "" {
		cfg.Name = cfg.Path
	}

	if pluginName == "" {
		// use embed fs objstore
		st, err = filestore.Open(cfg, false)
	} else {
		// use plugin objstore
		st, err = objstoreplugin.OpenPluginObjStore(pluginName, cfg, loadedPlugins)
	}

	if err != nil {
		return
	}
	log.Infow("store constructed", "type", st.Type(), "ver", st.Version(), "instance", st.Instance(context.Background()))

	return
}

func BuildPersistedFileStoreMgr(scfg *modules.SafeConfig, globalStore CommonMetaStore, loadedPlugins *managerplugin.LoadedPlugins) (PersistedObjectStoreManager, error) {
	persistCfg := scfg.MustCommonConfig().PersistStores

	stores := make([]objstore.Store, 0, len(persistCfg))
	policy := map[string]objstore.StoreSelectPolicy{}
	for pi := range persistCfg {
		// For compatibility with v0.5
		if persistCfg[pi].PluginName == "" && persistCfg[pi].Plugin != "" {
			persistCfg[pi].PluginName = persistCfg[pi].Plugin
		}
		st, err := openObjStore(persistCfg[pi].Config, persistCfg[pi].PluginName, loadedPlugins)
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

func BuildSectorTracker(indexer core.SectorIndexer, state core.SectorStateManager, prover core.Prover, capi chain.API, scfg *modules.SafeConfig) (core.SectorTracker, error) {
	return sectors.NewTracker(indexer, state, prover, capi, scfg.MustCommonConfig().Proving)
}

type MarketAPIRelatedComponents struct {
	fx.Out

	DealManager core.DealManager
	MarketAPI   market.API
	PieceStore  piecestore.PieceStore
}

func BuildMarketAPI(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig) (market.API, error) {
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

func BuildMarketAPIRelated(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, minerAPI core.MinerAPI, loadedPlugins *managerplugin.LoadedPlugins) (MarketAPIRelatedComponents, error) {
	mapi, err := BuildMarketAPI(gctx, lc, scfg)
	if err != nil {
		return MarketAPIRelatedComponents{}, fmt.Errorf("build market api: %w", err)
	}

	if mapi == nil {
		log.Warn("deal manager based on market api is disabled, use mocked")
		return MarketAPIRelatedComponents{
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
			ReadOnly: pcfg.ReadOnly,
		}
		// For compatibility with v0.5
		if pcfg.PluginName == "" && pcfg.Plugin != "" {
			pcfg.PluginName = pcfg.Plugin
		}
		st, err := openObjStore(cfg, pcfg.PluginName, loadedPlugins)
		if err != nil {
			return MarketAPIRelatedComponents{}, fmt.Errorf("construct #%d piece store: %w", pi, err)
		}

		stores = append(stores, st)
	}

	proxy := piecestore.NewProxy(stores, mapi)
	http.DefaultServeMux.Handle(HTTPEndpointPiecestore, http.StripPrefix(HTTPEndpointPiecestore, proxy))
	log.Info("piecestore proxy has been registered into default mux")

	return MarketAPIRelatedComponents{
		DealManager: dealmgr.New(mapi, minerAPI, scfg),
		MarketAPI:   mapi,
		PieceStore:  proxy,
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
	minerAPI core.MinerAPI,
	stateMgr core.SectorStateManager,
	store SnapUpMetaStore,
) (core.SnapUpSectorManager, error) {
	mgr, err := sectors.NewSnapUpMgr(gctx, tracker, indexer, chainAPI, eventbus, messagerAPI, minerAPI, stateMgr, scfg, store)
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

func BuildRebuildManager(
	gctx GlobalContext,
	db UnderlyingDB,
	scfg *modules.SafeConfig,
	minerAPI core.MinerAPI,
) (core.RebuildSectorManager, error) {
	store, err := db.OpenCollection(gctx, "rebuild")
	if err != nil {
		return nil, err
	}

	mgr, err := sectors.NewRebuildManager(scfg, minerAPI, store)
	if err != nil {
		return nil, fmt.Errorf("construct rebuild manager: %w", err)
	}
	return mgr, nil
}

func BuildWorkerMetaStore(gctx GlobalContext, db UnderlyingDB) (WorkerMetaStore, error) {
	return db.OpenCollection(gctx, "worker")
}

func BuildWorkerManager(meta WorkerMetaStore) (core.WorkerManager, error) {
	return worker.NewManager(meta)
}

func BuildProxiedSectorIndex(client *core.SealerCliAPIClient, storeMgr PersistedObjectStoreManager) (core.SectorIndexer, error) {
	log.Debug("build proxied sector indexer")
	return sectors.NewProxiedIndexer(client, storeMgr)
}

func BuildUnsealManager(
	gctx GlobalContext,
	db UnderlyingDB,
	scfg *modules.SafeConfig,
	MinerAPI core.MinerAPI,
) (core.UnsealSectorManager, error) {
	store, err := db.OpenCollection(gctx, "unseal")
	if err != nil {
		return nil, err
	}

	mgr, err := sectors.NewUnsealManager(gctx, scfg, MinerAPI, store)
	if err != nil {
		return nil, fmt.Errorf("construct unseal manager: %w", err)
	}
	return mgr, nil
}

func BuildWorkerProverStore(gctx GlobalContext, db UnderlyingDB) (WorkerProverStore, error) {
	return db.OpenCollection(gctx, "prover")
}

func BuildWorkerWdPoStTaskManager(kv WorkerProverStore) (core.WorkerWdPoStTaskManager, error) {
	wdpostKV, err := kvstore.NewWrappedKVStore([]byte("wdpost-"), kv)
	if err != nil {
		return nil, err
	}
	return proverworker.NewKVTaskManager(*kvstore.NewKVExt(wdpostKV)), nil
}
