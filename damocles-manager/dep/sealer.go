package dep

import (
	"context"
	"sync"

	"github.com/dtynn/dix"
	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/mock"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover"
	proverworker "github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover/worker"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/randomness"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/sealer"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/confmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/market"
	messager "github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type GlobalContext context.Context

func Mock() dix.Option {
	return dix.Options(
		dix.Override(new(core.RandomnessAPI), mock.NewRandomness),
		dix.Override(new(core.SectorManager), mock.NewSectorManager),
		dix.Override(new(core.DealManager), mock.NewDealManager),
		dix.Override(new(core.CommitmentManager), mock.NewCommitManager),
		dix.Override(new(core.MinerAPI), mock.NewMinerAPI),
	)
}

func MockSealer(s ...interface{}) dix.Option {
	return dix.Options(
		dix.Override(new(*mock.Sealer), mock.NewSealer),
		dix.Override(new(core.SealerAPI), dix.From(new(*mock.Sealer))),
		dix.Populate(InvokePopulate, s...),
	)
}

func Product() dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(ConfDirPath), BuildConfDirPath),
		dix.Override(new(*modules.Config), ProvideConfig),
		dix.Override(new(*modules.SafeConfig), ProvideSafeConfig),
		dix.Override(new(*managerplugin.LoadedPlugins), ProvidePlugins),
		dix.Override(new(UnderlyingDB), BuildUnderlyingDB),
		dix.Override(new(core.SectorManager), BuildLocalSectorManager),
		dix.Override(new(core.SectorStateManager), BuildLocalSectorStateManager),
		dix.Override(new(core.SectorNumberAllocator), BuildSectorNumberAllocator),
		dix.Override(new(core.RandomnessAPI), randomness.New),
		dix.Override(new(core.SectorTracker), BuildSectorTracker),
		dix.Override(new(core.Prover), prover.Prover),
		dix.Override(new(core.Verifier), prover.Verifier),
		dix.Override(new(core.MinerAPI), BuildMinerAPI),

		dix.Override(new(core.CommitmentManager), BuildCommitmentManager),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(PersistedObjectStoreManager), BuildPersistedFileStoreMgr),
		dix.Override(new(core.SectorIndexer), BuildSectorIndexer),
		dix.Override(new(*chain.EventBus), BuildChainEventBus),

		dix.Override(ConstructMarketAPIRelated, BuildMarketAPIRelated),
		dix.Override(new(core.WorkerManager), BuildWorkerManager),

		dix.Override(new(core.SnapUpSectorManager), BuildSnapUpManager),
		dix.Override(new(core.RebuildSectorManager), BuildRebuildManager),
		dix.Override(new(core.UnsealSectorManager), BuildUnsealManager),

		dix.Override(new(SnapUpMetaStore), BuildSnapUpMetaStore),
		dix.Override(new(SectorIndexMetaStore), BuildSectorIndexMetaStore),
		dix.Override(new(OnlineMetaStore), BuildOnlineMetaStore),
		dix.Override(new(OfflineMetaStore), BuildOfflineMetaStore),
		dix.Override(new(WorkerMetaStore), BuildWorkerMetaStore),
		dix.Override(new(CommonMetaStore), BuildCommonMetaStore),
		dix.Override(new(WorkerProverStore), BuildWorkerProverStore),

		dix.Override(new(core.WorkerWdPoStTaskManager), BuildWorkerWdPoStTaskManager),
		dix.Override(new(core.WorkerWdPoStAPI), proverworker.NewWdPoStAPIImpl),
	)
}

type ProxyOptions struct {
	EnableSectorIndexer bool
}

func Proxy(dest string, opt ProxyOptions) dix.Option {
	return dix.Options(
		dix.Override(new(ProxyAddress), ProxyAddress(dest)),
		dix.Override(new(core.APIClient), BuildAPIProxyClient),
		dix.Override(new(core.SealerCliAPIClient), BuildSealerCliAPIClient),
		dix.If(opt.EnableSectorIndexer,
			dix.Override(new(core.SectorIndexer), BuildProxiedSectorIndex),
		),
	)
}

func Sealer(target ...interface{}) dix.Option {
	return dix.Options(
		dix.Override(new(*sealer.Sealer), sealer.New),
		dix.Override(new(core.SealerAPI), dix.From(new(*sealer.Sealer))),
		dix.If(len(target) > 0, dix.Populate(InvokePopulate, target...)),
	)
}

func APIClient(target ...interface{}) dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(ConfDirPath), BuildConfDirPath),
		dix.Override(new(*modules.Config), ProvideConfig),
		dix.Override(new(*modules.SafeConfig), ProvideSafeConfig),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(market.API), BuildMarketAPI),
		dix.Override(new(core.APIClient), MaybeAPIClient),
		dix.Override(new(core.SealerCliAPIClient), BuildSealerCliAPIClient),
		dix.If(len(target) > 0, dix.Populate(InvokePopulate, target...)),
	)
}
