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
		dix.Override(InjectSealerAPI, func(instance *mock.Sealer) error {
			*s = instance
			return nil
		}),
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
		dix.Override(new(api.Prover), prover.Prover),
		dix.Override(new(api.Verifier), prover.Verifier),
		dix.Override(new(api.MinerInfoAPI), BuildMinerInfoAPI),

		dix.Override(new(api.CommitmentManager), BuildCommitmentManager),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(PersistedObjectStoreManager), BuildPersistedFileStoreMgr),
		dix.Override(new(SectorIndexMetaStore), BuildSectorIndexMetaStore),
		dix.Override(new(api.SectorIndexer), BuildSectorIndexer),

		// TODO: use functional deal manager
		dix.Override(new(api.DealManager), mock.NewDealManager),
	)
}

func Sealer(s *api.SealerAPI) dix.Option {
	return dix.Options(
		dix.Override(new(*sealer.Sealer), sealer.New),
		dix.Override(InjectSealerAPI, func(instance *sealer.Sealer) error {
			*s = instance
			return nil
		}),
	)
}

func API(c *chain.API, m *messager.API) dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(*modules.Config), ProvideConfig),
		dix.Override(new(chain.API), BuildChainClient),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.If(c != nil,
			dix.Override(InjectChainAPI, func(instance chain.API) error {
				*c = instance
				return nil
			}),
		),
		dix.If(m != nil,
			dix.Override(InjectMessagerAPI, func(instance messager.API) error {
				*m = instance
				return nil
			}),
		),
	)
}

func SealerClient(s *api.SealerClient) dix.Option {
	return dix.Options(
		dix.Override(new(api.SealerClient), BuildSealerClient),
		dix.Override(InjectSealerClient, func(instance api.SealerClient) error {
			*s = instance
			return nil
		}),
	)
}
