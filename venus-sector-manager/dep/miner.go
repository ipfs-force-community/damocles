package dep

import (
	"fmt"

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

func StartProofEvent(gctx GlobalContext, lc fx.Lifecycle, prover api.Prover, cfg *modules.SafeConfig, indexer api.SectorIndexer) error {
	cfg.Lock()
	actors := cfg.RegisterProof.Actors
	cfg.Unlock()

	for key, rpCfg := range actors {
		mid, err := modules.ActorIDFromConfigKey(key)
		if err != nil {
			return fmt.Errorf("parse actor id from %s: %w", key, err)
		}

		maddr, err := address.NewIDAddress(uint64(mid))
		if err != nil {
			return err
		}

		actor := api.ActorIdent{
			Addr: maddr,
			ID:   mid,
		}

		for _, addr := range rpCfg.Apis {
			client, err := miner.NewProofEventClient(lc, addr, rpCfg.Token)
			if err != nil {
				return err
			}

			proofEvent := miner.NewProofEvent(prover, client, actor, indexer)

			go proofEvent.StartListening(gctx)
		}
	}

	return nil
}
