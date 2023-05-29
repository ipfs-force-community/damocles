package dep

import (
	"context"
	"fmt"

	"github.com/dtynn/dix"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus/venus-shared/api/gateway/v2"
	"go.uber.org/fx"

	mkapi "github.com/filecoin-project/venus/venus-shared/api/market/v1"
	assets "github.com/ipfs-force-community/venus-cluster-assets"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	market "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/miner"
)

type GatewayClients []gateway.IGateway
type MarketEventClients []gateway.IMarketServiceProvider
type WinningPoStWarmUp bool

func Gateway() dix.Option {
	return dix.Options(
		dix.Override(new(GatewayClients), NewGatewayClients),
		dix.Override(new(MarketEventClients), NewMarketEventClients),
		dix.Override(MarketEventInvoke, StartMarketEvent),
	)
}

func Miner() dix.Option {
	return dix.Options(
		dix.Override(ProofEventInvoke, StartProofEvent),
	)
}

func NewGatewayClients(gctx GlobalContext, lc fx.Lifecycle, cfg *modules.SafeConfig) (GatewayClients, error) {
	cfg.Lock()
	urls, commonToken := cfg.Common.API.Gateway, cfg.Common.API.Token
	cfg.Unlock()

	var ret GatewayClients

	for _, u := range urls {
		addr, token := extractAPIInfo(u, commonToken)

		client, closer, err := gateway.DialIGatewayRPC(context.Background(), addr, token, nil, jsonrpc.WithRetry(true))
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				closer()
				return nil
			},
		})

		ret = append(ret, client)
	}

	return ret, nil
}

func NewMarketEventClients(gctx GlobalContext, lc fx.Lifecycle, cfg *modules.SafeConfig, gatewayClients GatewayClients) (MarketEventClients, error) {
	cfg.Lock()
	addr, token := cfg.Common.API.Market, cfg.Common.API.Token
	cfg.Unlock()

	var ret MarketEventClients

	cli, closer, err := mkapi.DialIMarketRPC(context.Background(), addr, token, nil, jsonrpc.WithRetry(true))
	if err != nil {
		return nil, fmt.Errorf("construct market api client: %w", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			closer()
			return nil
		},
	})

	ret = append(ret, cli)
	for _, cli := range gatewayClients {
		ret = append(ret, cli)
	}

	return ret, nil
}

func StartProofEvent(gctx GlobalContext, prover core.Prover, cfg *modules.SafeConfig, tracker core.SectorTracker, warmup WinningPoStWarmUp, gClients GatewayClients) error {
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
	miners := cfg.Miners
	cfg.Unlock()

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

	for _, client := range gClients {
		for _, actor := range actors {
			proofEvent := miner.NewProofEvent(prover, client, actor, tracker)
			go proofEvent.StartListening(gctx)
		}
	}

	return nil
}

func StartMarketEvent(gctx GlobalContext, cfg *modules.SafeConfig, unseal core.UnsealSectorManager, mClients MarketEventClients) error {

	cfg.Lock()
	miners := cfg.Miners
	cfg.Unlock()

	actors := make([]core.ActorIdent, 0, len(miners))

	for _, mcfg := range miners {
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

	for _, client := range mClients {
		for _, actor := range actors {
			proofEvent := market.New(unseal, client, actor.Addr)
			go proofEvent.StartListening(gctx)
		}
	}

	return nil
}
