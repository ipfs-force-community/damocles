package miner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	v1 "github.com/filecoin-project/venus/venus-shared/api/gateway/v1"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var log = logging.Logger("proof_event")

type ProofEvent struct {
	prover  api.Prover
	client  v1.IGateway
	actor   api.ActorIdent
	indexer api.SectorIndexer
}

func NewProofEvent(prover api.Prover, client v1.IGateway, actor api.ActorIdent, indexer api.SectorIndexer) *ProofEvent {
	pe := &ProofEvent{
		prover:  prover,
		client:  client,
		actor:   actor,
		indexer: indexer,
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
		return xerrors.Errorf("listenHeadChanges ChainNotify call failed: %w", err)
	}

	for proofEvent := range proofEventCh {
		switch proofEvent.Method {
		case "InitConnect":
			req := gtypes.ConnectedCompleted{}
			err := json.Unmarshal(proofEvent.Payload, &req)
			if err != nil {
				return xerrors.Errorf("odd error in connect %v", err)
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
	out := make([]api.PrivateSectorInfo, 0, len(sectorInfo))
	for _, sector := range sectorInfo {
		sid := storage.SectorRef{
			ID:        abi.SectorID{Miner: pe.actor.ID, Number: sector.SectorNumber},
			ProofType: sector.SealProof,
		}

		postProofType, err := sid.ProofType.RegisteredWinningPoStProof()
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("acquiring registered PoSt proof from sector info %+v: %w", sectorInfo, err)
		}

		objins, err := pe.getObjInstanceForSector(ctx, sid.ID)
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("get objstore instance for %s: %w", util.FormatSectorID(sid.ID), err)
		}

		// TODO: Construct paths for snap deals ?
		proveUpdate := sector.SectorKey != nil
		var (
			subCache, subSealed string
		)
		if proveUpdate {

		} else {
			subCache = util.SectorPath(util.SectorPathTypeCache, sid.ID)
			subSealed = util.SectorPath(util.SectorPathTypeSealed, sid.ID)
		}

		ffiInfo := builtin.SectorInfo{
			SealProof:    sector.SealProof,
			SectorNumber: sector.SectorNumber,
			SealedCID:    sector.SealedCID,
		}
		out = append(out, api.PrivateSectorInfo{
			CacheDirPath:     objins.FullPath(ctx, subCache),
			PoStProofType:    postProofType,
			SealedSectorPath: objins.FullPath(ctx, subSealed),
			SectorInfo:       ffiInfo,
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
