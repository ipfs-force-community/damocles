package dep

import (
	"fmt"

	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/miner"
)

func Miner() dix.Option {
	return dix.Options(
		dix.Override(StartMiner, StartProofEvent),
	)
}

func StartProofEvent(gctx GlobalContext, lc fx.Lifecycle, prover core.Prover, cfg *modules.SafeConfig, tracker core.SectorTracker) error {
	cfg.Lock()
	urls, token, miners := cfg.Common.API.Gateway, cfg.Common.API.Token, cfg.Miners
	cfg.Unlock()

	if len(urls) == 0 {
		return fmt.Errorf("no gateway api addr provided")
	}

	actors := make([]core.ActorIdent, 0, len(miners))

	for _, mcfg := range miners {
		if !mcfg.Proof.Enabled {
			continue
		}

		maddr, err := address.NewIDAddress(uint64(mcfg.Actor))
		if err != nil {
			return err
		}

		actors = append(actors, core.ActorIdent{
			Addr: maddr,
			ID:   mcfg.Actor,
		})
	}

	if len(actors) == 0 {
		return nil
	}

	for _, addr := range urls {
		client, err := miner.NewProofEventClient(lc, addr, token)
		if err != nil {
			return err
		}

		for _, actor := range actors {
			proofEvent := miner.NewProofEvent(prover, client, actor, tracker)
			go proofEvent.StartListening(gctx)
		}
	}

	return nil
}
