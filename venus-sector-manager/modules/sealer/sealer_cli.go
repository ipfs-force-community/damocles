package sealer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

func (s *Sealer) ListSectors(ctx context.Context, ws core.SectorWorkerState) ([]*core.SectorState, error) {
	return s.state.All(ctx, ws, core.SectorWorkerJobSealing)
}

func (s *Sealer) RestoreSector(ctx context.Context, sid abi.SectorID, forced bool) (core.Meta, error) {
	var onRestore func(st *core.SectorState) (bool, error)
	if !forced {
		onRestore = func(st *core.SectorState) (bool, error) {
			if len(st.Pieces) != 0 {
				return false, fmt.Errorf("sector with deals can not be normally restored")
			}

			if st.AbortReason == "" {
				return false, fmt.Errorf("sector is not aborted, can not be normally restored")
			}

			st.AbortReason = ""
			return true, nil
		}
	}

	err := s.state.Restore(ctx, sid, onRestore)
	if err != nil {
		return core.Empty, err
	}

	return core.Empty, nil
}

func (s *Sealer) CheckProvable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
	return s.sectorTracker.Provable(ctx, mid, sectors, strict)
}

func (s *Sealer) SimulateWdPoSt(ctx context.Context, maddr address.Address, sis []builtin.ExtendedSectorInfo, rand abi.PoStRandomness) error {
	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return err
	}

	privSectors, err := s.sectorTracker.PubToPrivate(ctx, abi.ActorID(mid), sis, core.SectorWindowPoSt)
	if err != nil {
		return fmt.Errorf("turn public sector infos into private: %w", err)
	}

	go func() {
		tCtx := context.TODO()

		tsStart := clock.NewSystemClock().Now()

		log.Info("mock generate window post start")
		_, _, err = s.prover.GenerateWindowPoSt(tCtx, abi.ActorID(mid), core.NewSortedPrivateSectorInfo(privSectors...), append(abi.PoStRandomness{}, rand...))
		if err != nil {
			log.Warnf("generate window post failed: %v", err.Error())
			return
		}

		elapsed := time.Since(tsStart)
		log.Infow("mock generate window post", "elapsed", elapsed)
	}()

	return nil
}

func (s *Sealer) SnapUpPreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*core.SnapUpFetchResult, error) {
	count, diff, err := s.snapup.PreFetch(ctx, mid, dlindex)
	if err != nil {
		return nil, fmt.Errorf("prefetch: %w", err)
	}

	return &core.SnapUpFetchResult{
		Total: count,
		Diff:  diff,
	}, nil
}

func (s *Sealer) SnapUpCandidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error) {
	return s.snapup.Candidates(ctx, mid)
}

func (s *Sealer) ProvingSectorInfo(ctx context.Context, sid abi.SectorID) (core.ProvingSectorInfo, error) {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return core.ProvingSectorInfo{}, fmt.Errorf("invalid mienr actor id: %w", err)
	}

	sinfo, err := s.capi.StateSectorGetInfo(ctx, maddr, sid.Number, types.EmptyTSK)
	if err != nil {
		return core.ProvingSectorInfo{}, fmt.Errorf("get sector info: %w", err)
	}

	private, err := s.sectorTracker.SinglePubToPrivateInfo(ctx, sid.Miner, util.SectorOnChainInfoToExtended(sinfo), nil)
	if err != nil {
		return core.ProvingSectorInfo{}, fmt.Errorf("get private sector info: %w", err)
	}

	return core.ProvingSectorInfo{
		OnChain: *sinfo,
		Private: private,
	}, nil

}

func (s *Sealer) WorkerGetPingInfo(ctx context.Context, name string) (*core.WorkerPingInfo, error) {
	winfo, err := s.workerMgr.Load(ctx, name)
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("load worker info: %w", err)
	}

	return &winfo, nil
}

func (s *Sealer) WorkerPingInfoList(ctx context.Context) ([]core.WorkerPingInfo, error) {
	winfos, err := s.workerMgr.All(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("load all worker infos: %w", err)
	}

	return winfos, nil
}
