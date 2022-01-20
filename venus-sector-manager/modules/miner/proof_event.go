package miner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/filecoin-project/venus/pkg/specactors/builtin"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
	"time"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/ipfs-force-community/venus-gateway/proofevent"
	"github.com/ipfs-force-community/venus-gateway/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

var log = logging.Logger("proof_event")

type ProofEvent struct {
	prover  api.Prover
	client  proofevent.IProofEventAPI
	actor   api.ActorIdent
	indexer api.SectorIndexer
}

func NewProofEvent(prover api.Prover, client proofevent.IProofEventAPI, actor api.ActorIdent, indexer api.SectorIndexer) *ProofEvent {
	pe := &ProofEvent{
		prover:  prover,
		client:  client,
		actor:   actor,
		indexer: indexer,
	}

	return pe
}

func (pe *ProofEvent) StartListening(ctx context.Context) {
	log.Infof("start proof event listening")
	for {
		if err := pe.listenProofRequestOnce(ctx); err != nil {
			log.Errorf("listen head changes errored: %s", err)
		} else {
			log.Warn("listenHeadChanges quit")
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			log.Warnf("not restarting listenHeadChanges: context error: %s", ctx.Err())
			return
		}

		log.Info("restarting listenHeadChanges")
	}
}

func (pe *ProofEvent) listenProofRequestOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	policy := &proofevent.ProofRegisterPolicy{
		MinerAddress: address.Address(pe.actor.Addr),
	}
	proofEventCh, err := pe.client.ListenProofEvent(ctx, policy)
	if err != nil {
		// Retry is handled by caller
		return xerrors.Errorf("listenHeadChanges ChainNotify call failed: %w", err)
	}

	for proofEvent := range proofEventCh {
		switch proofEvent.Method {
		case "InitConnect":
			req := types.ConnectedCompleted{}
			err := json.Unmarshal(proofEvent.Payload, &req)
			if err != nil {
				return xerrors.Errorf("odd error in connect %v", err)
			}
			log.Infof("success to connect with proof %s", req.ChannelId)
		case "ComputeProof":
			req := types.ComputeProofRequest{}
			err := json.Unmarshal(proofEvent.Payload, &req)
			if err != nil {
				_ = pe.client.ResponseProofEvent(ctx, &types.ResponseEvent{
					Id:      proofEvent.Id,
					Payload: nil,
					Error:   err.Error(),
				})
				continue
			}
			pe.processComputeProof(ctx, proofEvent.Id, req)
		default:
			log.Errorf("unexpect proof event type %s", proofEvent.Method)
		}
	}

	return nil
}

// context.Context, []builtin.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version
func (pe *ProofEvent) processComputeProof(ctx context.Context, reqId uuid.UUID, req types.ComputeProofRequest) {
	privSectors, err := pe.sectorsPubToPrivate(ctx, req.SectorInfos)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &types.ResponseEvent{
			Id:      reqId,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	proof, err := pe.prover.GenerateWinningPoSt(ctx, pe.actor.ID, privSectors, req.Rand)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &types.ResponseEvent{
			Id:      reqId,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	proofBytes, err := json.Marshal(proof)
	if err != nil {
		_ = pe.client.ResponseProofEvent(ctx, &types.ResponseEvent{
			Id:      reqId,
			Payload: nil,
			Error:   err.Error(),
		})
		return
	}

	err = pe.client.ResponseProofEvent(ctx, &types.ResponseEvent{
		Id:      reqId,
		Payload: proofBytes,
		Error:   "",
	})
	if err != nil {
		log.Errorf("response proof event %s failed", reqId)
	}
}

func (pe *ProofEvent) sectorsPubToPrivate(ctx context.Context, sectorInfo []builtin.SectorInfo) (api.SortedPrivateSectorInfo, error) {
	out := make([]api.PrivateSectorInfo, 0, len(sectorInfo))
	for _, sector := range sectorInfo {
		sid := storage.SectorRef{
			ID:        abi.SectorID{Miner: pe.actor.ID, Number: sector.SectorNumber},
			ProofType: sector.SealProof,
		}

		postProofType, err := sid.ProofType.RegisteredWinningPoStProof()
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("acquiring registered PoSt proof from sector info: %w", err)
		}

		objins, err := pe.getObjInstanceForSector(ctx, sid.ID)
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("get objstore instance for %s: %w", util.FormatSectorID(sid.ID), err)
		}

		subCache := util.SectorPath(util.SectorPathTypeCache, sid.ID)
		subSealed := util.SectorPath(util.SectorPathTypeSealed, sid.ID)

		out = append(out, api.PrivateSectorInfo{
			CacheDirPath:     objins.FullPath(ctx, subCache),
			PoStProofType:    postProofType,
			SealedSectorPath: objins.FullPath(ctx, subSealed),
			SectorInfo:       sector,
		})
	}

	return api.NewSortedPrivateSectorInfo(out...), nil
}

func (pe *ProofEvent) getObjInstanceForSector(ctx context.Context, sid abi.SectorID) (objstore.Store, error) {
	insname, has, err := pe.indexer.Find(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find objstore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("objstore instance not found")
	}

	instance, err := pe.indexer.StoreMgr().GetInstance(ctx, insname)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s: %w", insname, err)
	}

	return instance, nil
}
