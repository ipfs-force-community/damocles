package dep

import (
	"context"
	"sync"

	"github.com/dtynn/dix"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	messager "github.com/dtynn/venus-cluster/venus-sealer/pkg/messager"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/mock"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/prover"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/randomness"
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
		dix.Override(new(*sealer.Config), ProvideSealerConfig),
		dix.Override(new(api.SectorManager), BuildLocalSectorManager),
		dix.Override(new(api.SectorStateManager), BuildLocalSectorStateManager),
		dix.Override(new(MetaStore), BuildMetaStore),
		dix.Override(new(api.SectorNumberAllocator), BuildSectorNumberAllocator),
		dix.Override(new(api.RandomnessAPI), randomness.New),
		dix.Override(new(api.Prover), prover.Prover),
		dix.Override(new(api.Verifier), prover.Verifier),
		dix.Override(new(api.MinerInfoAPI), chain.NewMinerInfoAPI),

		dix.Override(new(api.CommitmentManager), BuildCommitmentManager),
		dix.Override(new(messager.API), BuildMessagerClient),
		dix.Override(new(chain.API), BuildChainClient),

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
