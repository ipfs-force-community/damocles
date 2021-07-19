package dep

import (
	"context"
	"sync"

	"github.com/dtynn/dix"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	message_client "github.com/dtynn/venus-cluster/venus-sealer/pkg/message-client"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/commitmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/mock"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/prover"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/randomness"
)

func Mock() dix.Option {
	return dix.Options(
		dix.Override(new(api.RandomnessAPI), mock.NewRandomness),
		dix.Override(new(api.SectorManager), mock.NewSectorManager),
		dix.Override(new(api.DealManager), mock.NewDealManager),
		//dix.Override(new(api.CommitmentManager), mock.NewCommitManager),
		dix.Override(new(api.MinerInfoAPI), mock.NewMinerInfoAPI),

		// commit manager di
		dix.Override(new(commitmgr.Verifier), nil),
		dix.Override(new(commitmgr.Prover), nil),
		dix.Override(new(api.SectorsDatastore), nil),
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
		dix.Override(new(api.RandomnessAPI), randomness.New),
		dix.Override(new(api.Prover), prover.Prover),
		dix.Override(new(api.Verifier), prover.Verifier),

		dix.Override(new(api.CommitmentManager), commitmgr.NewCommitmentMgr),
		dix.Override(new(venusMessager.IMessager), message_client.NewMessageClient),
		dix.Override(new(commitmgr.SealingAPI), commitmgr.NewSealingAPIImpl),

		// TODO: make the dependencies below available
		dix.Override(new(chain.API), func() (chain.API, error) { return nil, nil }),
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
