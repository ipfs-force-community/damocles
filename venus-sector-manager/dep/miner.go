package dep

import (
	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/miner"
)

func Miner() dix.Option {
	return dix.Options(
		dix.Override(StartMiner, StartProofEvent),
	)
}

func StartProofEvent(gctx GlobalContext, lc fx.Lifecycle, prover api.Prover, cfg *modules.Config, indexer api.SectorIndexer) error {
	for _, m := range cfg.SectorManager.Miners {
		maddr, err := address.NewIDAddress(uint64(m.ID))
		if err != nil {
			return err
		}

		actor := api.ActorIdent{
			Addr: maddr,
			ID:   m.ID,
		}

		for _, addr := range cfg.RegisterProof.Apis {
			client, err := miner.NewProofEventClient(lc, addr, cfg.RegisterProof.Token)
			if err != nil {
				return err
			}

			proofEvent := miner.NewProofEvent(prover, client, actor, indexer)

			go proofEvent.StartListening(gctx)
		}
	}

	return nil
}
