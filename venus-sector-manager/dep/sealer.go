package dep

import (
	"context"
	"sync"

	"github.com/dtynn/dix"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/mock"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/randomness"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/sealer"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
	messager "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

type GlobalContext context.Context

func Mock() dix.Option {
	return dix.Options(
		dix.Override(new(api.RandomnessAPI), mock.NewRandomness),
		dix.Override(new(api.SectorManager), mock.NewSectorManager),
		dix.Override(new(api.DealManager), mock.NewDealManager),
		dix.Override(new(api.CommitmentManager), mock.NewCommitManager),
	)
}

func MockSealer(s *api.SealerAPI) dix.Option {
	return dix.Options(
		dix.Override(new(*mock.Sealer), mock.NewSealer),
		dix.Override(new(api.SealerAPI), dix.From(new(*mock.Sealer))),
		dix.Populate(InvokePopulate, s),
	)
}

func Product() dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(*modules.Config), ProvideConfig),
		dix.Override(new(*modules.SafeConfig), ProvideSafeConfig),
		dix.Override(new(api.SectorManager), BuildLocalSectorManager),
		dix.Override(new(api.SectorStateManager), BuildLocalSectorStateManager),
		dix.Override(new(OnlineMetaStore), BuildOnlineMetaStore),
		dix.Override(new(OfflineMetaStore), BuildOfflineMetaStore),
		dix.Override(new(api.SectorNumberAllocator), BuildSectorNumberAllocator),
		dix.Override(new(api.RandomnessAPI), randomness.New),
		dix.Override(new(api.SectorTracker), BuildSectorTracker),
		dix.Override(new(api.Prover), prover.Prover),
		dix.Override(new(api.Verifier), prover.Verifier),
		dix.Override(new(api.MinerInfoAPI), BuildMinerInfoAPI),

		dix.Override(new(api.CommitmentManager), BuildCommitmentManager),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(PersistedObjectStoreManager), BuildPersistedFileStoreMgr),
		dix.Override(new(SectorIndexMetaStore), BuildSectorIndexMetaStore),
		dix.Override(new(api.SectorIndexer), BuildSectorIndexer),
		dix.Override(ConstructMarketAPIRelated, BuildMarketAPIRelated),
	)
}

func Sealer(target ...interface{}) dix.Option {
	return dix.Options(
		dix.Override(new(*sealer.Sealer), sealer.New),
		dix.Override(new(api.SealerAPI), dix.From(new(*sealer.Sealer))),
		dix.If(len(target) > 0, dix.Populate(InvokePopulate, target...)),
	)
}

func API(target ...interface{}) dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(*modules.Config), ProvideConfig),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(api.MinerInfoAPI), BuildMinerInfoAPI),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(market.API), BuildMarketAPI),
		dix.If(len(target) > 0, dix.Populate(InvokePopulate, target...)),
	)
}

func SealerClient(s *api.SealerClient) dix.Option {
	return dix.Options(
		dix.Override(new(api.SealerClient), BuildSealerClient),
		dix.Populate(InvokePopulate, s),
	)
}
