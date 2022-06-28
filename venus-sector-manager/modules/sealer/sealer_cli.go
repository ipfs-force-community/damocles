package sealer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

func (s *Sealer) ListSectors(ctx context.Context, ws core.SectorWorkerState, job core.SectorWorkerJob) ([]*core.SectorState, error) {
	return s.state.All(ctx, ws, job)
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

	slog := log.With("miner", mid, "sectors", len(privSectors))

	go func() {
		tCtx := context.TODO()

		tsStart := clock.NewSystemClock().Now()

		slog.Info("mock generate window post start")
		proof, skipped, err := s.prover.GenerateWindowPoSt(tCtx, abi.ActorID(mid), core.NewSortedPrivateSectorInfo(privSectors...), append(abi.PoStRandomness{}, rand...))
		if err != nil {
			slog.Warnf("generate window post failed: %v", err.Error())
			return
		}

		elapsed := time.Since(tsStart)
		slog.Infow("mock generate window post", "elapsed", elapsed, "proof-size", len(proof), "skipped", len(skipped))
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

func (s *Sealer) SnapUpCancelCommitment(ctx context.Context, sid abi.SectorID) error {
	s.snapup.CancelCommitment(ctx, sid)
	return nil
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

func (s *Sealer) SectorIndexerFind(ctx context.Context, indexType core.SectorIndexType, sid abi.SectorID) (core.SectorIndexLocation, error) {
	var indexer core.SectorTypedIndexer

	switch indexType {
	case core.SectorIndexTypeNormal:
		indexer = s.sectorIdxer.Normal()

	case core.SectorIndexTypeUpgrade:
		indexer = s.sectorIdxer.Upgrade()

	default:
		return core.SectorIndexLocation{}, fmt.Errorf("sector indexer of type %s is not supported", indexType)
	}

	instance, found, err := indexer.Find(ctx, sid)
	if err != nil {
		return core.SectorIndexLocation{}, fmt.Errorf("find in indexer of type %s: %w", indexType, err)
	}

	return core.SectorIndexLocation{
		Found:    found,
		Instance: instance,
	}, nil
}

func (s *Sealer) TerminateSector(ctx context.Context, sid abi.SectorID) (core.SubmitTerminateResp, error) {
	return s.commit.SubmitTerminate(ctx, sid)
}

func (s *Sealer) PollTerminateSectorState(ctx context.Context, sid abi.SectorID) (core.TerminateInfo, error) {
	return s.commit.TerminateState(ctx, sid)
}

func (s *Sealer) RemoveSector(ctx context.Context, sid abi.SectorID) error {
	state, err := s.state.Load(ctx, sid, core.WorkerOffline)
	if err != nil {
		return fmt.Errorf("load sector state: %w", err)
	}

	if state.Removed {
		return nil
	}

	if state.TerminateInfo.TerminatedAt > 0 {
		ts, err := s.capi.ChainHead(ctx)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
		}

		nv, err := s.capi.StateNetworkVersion(ctx, ts.Key())
		if err != nil {
			return fmt.Errorf("getting network version: %w", err)
		}

		if ts.Height() < state.TerminateInfo.TerminatedAt+specpolicy.GetWinningPoStSectorSetLookback(nv) {
			height := state.TerminateInfo.TerminatedAt + specpolicy.GetWinningPoStSectorSetLookback(nv)
			return fmt.Errorf("wait for expiration(+winning lookback?): %v", height)
		}
	}

	dest := s.sectorIdxer.Normal()
	if state.Upgraded {
		dest = s.sectorIdxer.Upgrade()
	}

	access, has, err := dest.Find(ctx, sid)
	if err != nil {
		return fmt.Errorf("find objstore instance: %w", err)
	}
	if !has {
		return fmt.Errorf("object not found")
	}

	sealedFile, err := s.sectorIdxer.StoreMgr().GetInstance(ctx, access.SealedFile)
	if err != nil {
		return fmt.Errorf("get objstore instance %s for sealed file: %w", access.SealedFile, err)
	}

	cacheDir, err := s.sectorIdxer.StoreMgr().GetInstance(ctx, access.CacheDir)
	if err != nil {
		return fmt.Errorf("get objstore instance %s for cache dir: %w", access.CacheDir, err)
	}

	var cache string
	var sealed string
	if state.Upgraded {
		cache = util.SectorPath(util.SectorPathTypeUpdateCache, state.ID)
		sealed = util.SectorPath(util.SectorPathTypeUpdate, state.ID)
	} else {
		cache = util.SectorPath(util.SectorPathTypeCache, state.ID)
		sealed = util.SectorPath(util.SectorPathTypeSealed, state.ID)
	}

	cachePath := cacheDir.FullPath(ctx, cache)
	err = os.RemoveAll(cachePath)
	if err != nil {
		return fmt.Errorf("remove cache: %w", err)
	}

	sealedPath := sealedFile.FullPath(ctx, sealed)
	err = os.Remove(sealedPath)
	if err != nil {
		return fmt.Errorf("remove sealed file: %w", err)
	}

	state.Removed = true
	err = s.state.Update(ctx, state.ID, core.WorkerOffline, state.Removed)
	if err != nil {
		return fmt.Errorf("update sector Removed failed: %w", err)
	}

	return nil
}

func (s *Sealer) StoreReleaseReserved(ctx context.Context, sid abi.SectorID) (bool, error) {
	done, err := s.sectorIdxer.StoreMgr().ReleaseReserved(ctx, util.FormatSectorID(sid))
	if err != nil {
		return false, fmt.Errorf("release reserved: %w", err)
	}

	return done, nil
}

func (s *Sealer) StoreList(ctx context.Context) ([]core.StoreDetailedInfo, error) {
	infos, err := s.sectorIdxer.StoreMgr().ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}

	details := make([]core.StoreDetailedInfo, 0, len(infos))
	for _, info := range infos {
		reservedBy := make([]core.ReservedItem, 0, len(info.Reserved.Reserved))
		for _, res := range info.Reserved.Reserved {
			reservedBy = append(reservedBy, res)
		}
		sort.Slice(reservedBy, func(i, j int) bool {
			if reservedBy[i].At != reservedBy[j].At {
				return reservedBy[i].At < reservedBy[j].At
			}

			return reservedBy[i].By < reservedBy[j].By
		})

		details = append(details, core.StoreDetailedInfo{
			StoreBasicInfo: storeConfig2StoreBasic(&info.Instance.Config),
			Type:           info.Instance.Type,
			Total:          info.Instance.Total,
			Free:           info.Instance.Free,
			Used:           info.Instance.Used,
			UsedPercent:    info.Instance.UsedPercent,
			Reserved:       info.Reserved.ReservedSize,
			ReservedBy:     reservedBy,
		})
	}

	return details, nil
}

func storeConfig2StoreBasic(ocfg *objstore.Config) core.StoreBasicInfo {
	return core.StoreBasicInfo{
		Name: ocfg.Name,
		Path: ocfg.Path,
		Meta: ocfg.Meta,
	}
}
