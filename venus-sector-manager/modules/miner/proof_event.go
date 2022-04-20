package miner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	v1 "github.com/filecoin-project/venus/venus-shared/api/gateway/v1"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("proof_event")

type ProofEvent struct {
	prover  api.Prover
	client  v1.IGateway
	actor   api.ActorIdent
	tracker api.SectorTracker
}

func NewProofEvent(prover api.Prover, client v1.IGateway, actor api.ActorIdent, tracker api.SectorTracker) *ProofEvent {
	pe := &ProofEvent{
		prover:  prover,
		client:  client,
		actor:   actor,
		tracker: tracker,
	}

	return pe
}

func (pe *ProofEvent) StartListening(ctx context.Context) {
	log.Infof("start proof event listening for %s", pe.actor.Addr)
	for {
		if err := pe.listenProofRequestOnce(ctx); err != nil {
			log.Errorf("%s listen head changes errored: %s", pe.actor.Addr, err)
		} else {
			log.Warnf(" %s listenHeadChanges quit", pe.actor.Addr)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			log.Warnf("%s not restarting listenHeadChanges: context error: %s", pe.actor.Addr, ctx.Err())
			return
		}

		log.Infof("restarting listenHeadChanges for %s", pe.actor.Addr)
	}
}

func (pe *ProofEvent) listenProofRequestOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	policy := &gtypes.ProofRegisterPolicy{
		MinerAddress: pe.actor.Addr,
	}

	proofEventCh, err := pe.client.ListenProofEvent(ctx, policy)
	if err != nil {
		// Retry is handled by caller
		return fmt.Errorf("listenHeadChanges ChainNotify call failed: %w", err)
	}

	for proofEvent := range proofEventCh {
		switch proofEvent.Method {
		case "InitConnect":
			req := gtypes.ConnectedCompleted{}
			err := json.Unmarshal(proofEvent.Payload, &req)
			if err != nil {
				return fmt.Errorf("odd error in connect %v", err)
			}
			log.Infof("%s success to connect with proof %s", pe.actor.Addr, req.ChannelId)
		case "ComputeProof":
			req := gtypes.ComputeProofRequest{}
			err := json.Unmarshal(proofEvent.Payload, &req)
			if err != nil {
				_ = pe.client.ResponseProofEvent(ctx, &gtypes.ResponseEvent{
					ID:      proofEvent.ID,
					Payload: nil,
					Error:   err.Error(),
				})
				continue
			}
			pe.processComputeProof(ctx, proofEvent.ID, req)
		default:
			log.Errorf("%s receive unexpect proof event type %s", pe.actor.Addr, proofEvent.Method)
		}
	}

	return nil
}

// context.Context, []builtin.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version
func (pe *ProofEvent) processComputeProof(ctx context.Context, reqID vtypes.UUID, req gtypes.ComputeProofRequest) {
	privSectors, err := pe.sectorsPubToPrivate(ctx, req.SectorInfos)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &gtypes.ResponseEvent{
			ID:      reqID,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	proof, err := pe.prover.GenerateWinningPoSt(ctx, pe.actor.ID, privSectors, req.Rand)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &gtypes.ResponseEvent{
			ID:      reqID,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	proofBytes, err := json.Marshal(proof)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &gtypes.ResponseEvent{
			ID:      reqID,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	err = pe.client.ResponseProofEvent(ctx, &gtypes.ResponseEvent{
		ID:      reqID,
		Payload: proofBytes,
		Error:   "",
	})
	if err != nil {
		log.Errorf("%s response proof event %s failed", pe.actor.Addr, reqID)
	}
}

func (pe *ProofEvent) sectorsPubToPrivate(ctx context.Context, sectorInfo []builtin.ExtendedSectorInfo) (api.SortedPrivateSectorInfo, error) {
	out, err := pe.tracker.PubToPrivate(ctx, pe.actor.ID, sectorInfo, api.SectorWinningPoSt)
	if err != nil {
		return api.SortedPrivateSectorInfo{}, fmt.Errorf("convert to private infos: %w", err)
	}

	return api.NewSortedPrivateSectorInfo(out...), nil
}
