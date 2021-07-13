package dep

import (
	"context"
	"sync"

	"github.com/dtynn/dix"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/mock"
)

func Mock() dix.Option {
	return dix.Options(
		dix.Override(new(api.RandomnessAPI), mock.NewRandomness),
		dix.Override(new(api.SectorManager), mock.NewSectorManager),
		dix.Override(new(api.DealManager), mock.NewDealManager),
		dix.Override(new(api.CommitmentManager), mock.NewCommitManager),
		dix.Override(new(api.MinerInfoAPI), mock.NewMinerInfoAPI),
	)
}

type GlobalContext context.Context

func Product() dix.Option {
	cfgmu := &sync.RWMutex{}
	return dix.Options(
		dix.Override(new(confmgr.WLocker), cfgmu),
		dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
		dix.Override(new(confmgr.ConfigManager), BuildLocalConfigManager),
		dix.Override(new(*sealer.Config), ProvideSealerConfig),
		dix.Override(new(api.SectorManager), BuildLocalSectorManager),
		dix.Override(new(MetaStore), BuildMetaStore),
		dix.Override(new(api.SectorNumberAllocator), BuildSectorNumberAllocator),
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
