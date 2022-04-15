package dep

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/BurntSushi/toml"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"
	vapi "github.com/filecoin-project/venus/venus-shared/api"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/commitmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/dealmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/mock"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/sectors"
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
	ListenAddress               string
)

func BuildLocalSectorManager(scfg *modules.SafeConfig, mapi api.MinerInfoAPI, numAlloc api.SectorNumberAllocator) (api.SectorManager, error) {
	return sectors.NewManager(scfg, mapi, numAlloc)
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

	log.Infof("Sector-manager initial cfg: %s\n", buf.String())

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
	api, token := scfg.Common.API.Messager, scfg.Common.API.Token
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

func MaybeSealerCliClient(gctx GlobalContext, lc fx.Lifecycle, listen ListenAddress) api.SealerCliClient {
	cli, err := BuildSealerCliClient(gctx, lc, listen)
	if err != nil {
		cli = api.UnavailableSealerCliClient
	}

	return cli
}

func BuildSealerCliClient(gctx GlobalContext, lc fx.Lifecycle, listen ListenAddress) (api.SealerCliClient, error) {
	var scli api.SealerCliClient

	addr, err := net.ResolveTCPAddr("tcp", string(listen))
	if err != nil {
		return scli, err
	}

	ip := addr.IP
	if ip == nil || ip.Equal(net.IPv4zero) {
		ip = net.IPv4(127, 0, 0, 1)
	}

	maddr := fmt.Sprintf("/ip4/%s/tcp/%d", ip, addr.Port)

	ainfo := vapi.NewAPIInfo(maddr, "")
	apiAddr, err := ainfo.DialArgs(vapi.VerString(api.MajorVersion))
	if err != nil {
		return scli, err
	}

	closer, err := jsonrpc.NewClient(gctx, apiAddr, "Venus", &scli, ainfo.AuthHeader())
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
	api, token := scfg.Common.API.Chain, scfg.Common.API.Token
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

func BuildMinerInfoAPI(gctx GlobalContext, lc fx.Lifecycle, capi chain.API, scfg *modules.Config, locker confmgr.RLocker) (api.MinerInfoAPI, error) {
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
	rapi api.RandomnessAPI,
	stmgr api.SectorStateManager,
	minfoAPI api.MinerInfoAPI,
	scfg *modules.SafeConfig,
	verif api.Verifier,
	prover api.Prover,
) (api.CommitmentManager, error) {
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
	persistCfg := scfg.Common.PersistStores
	locker.Unlock()

	return filestore.NewManager(persistCfg)
}

func BuildSectorIndexer(storeMgr PersistedObjectStoreManager, kv SectorIndexMetaStore) (api.SectorIndexer, error) {
	upgrade, err := kvstore.NewWrappedKVStore([]byte("sector-upgrade"), kv)
	if err != nil {
		return nil, fmt.Errorf("wrap kvstore for sector-upgrade: %w", err)
	}

	return sectors.NewIndexer(storeMgr, kv, upgrade)
}

func BuildSectorTracker(indexer api.SectorIndexer) (api.SectorTracker, error) {
	return sectors.NewTracker(indexer)
}

type MarketAPIRelatedComponets struct {
	fx.Out

	DealManager api.DealManager
	MarketAPI   market.API
}

func BuildMarketAPI(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, infoAPI api.MinerInfoAPI) (market.API, error) {
	scfg.Lock()
	api, token := scfg.Common.API.Market, scfg.Common.API.Token
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

func BuildMarketAPIRelated(gctx GlobalContext, lc fx.Lifecycle, scfg *modules.SafeConfig, infoAPI api.MinerInfoAPI) (MarketAPIRelatedComponets, error) {
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

	locals, err := filestore.OpenMany(pieceStoreCfg)
	if err != nil {
		return MarketAPIRelatedComponets{}, fmt.Errorf("open local piece stores: %w", err)
	}

	proxy := piecestore.NewProxy(locals, mapi)
	http.DefaultServeMux.Handle(HttpEndpointPiecestore, http.StripPrefix(HttpEndpointPiecestore, proxy))
	log.Info("piecestore proxy has been registered into default mux")

	return MarketAPIRelatedComponets{
		DealManager: dealmgr.New(mapi, infoAPI, scfg),
		MarketAPI:   mapi,
	}, nil
}
