package dep

import (
	"fmt"

	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	assets "github.com/ipfs-force-community/damocles-assets"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/miner"
)

type WinningPoStWarmUp bool

func Miner() dix.Option {
	return dix.Options(
		dix.Override(StartMiner, StartProofEvent),
	)
}

func StartProofEvent(gctx GlobalContext, lc fx.Lifecycle, prover core.Prover, cfg *modules.SafeConfig, tracker core.SectorTracker, warmup WinningPoStWarmUp) error {
	if warmup {
		log.Info("warm up for winning post")
		_, err := prover.GenerateWinningPoStWithVanilla(
			gctx,
			assets.WinningPoStWarmUp.PoStProof,
			assets.WinningPoStWarmUp.SectorID.Miner,
			assets.WinningPoStWarmUp.Randomness,
			[][]byte{assets.WinningPoStWarmUp.Vanilla},
		)
		if err != nil {
			return fmt.Errorf("warmup proof: %w", err)
		}

	}

	cfg.Lock()
	urls, commonToken, miners := cfg.Common.API.Gateway, cfg.Common.API.Token, cfg.Miners
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

	for _, u := range urls {
		addr, token := extractAPIInfo(u, commonToken)
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
