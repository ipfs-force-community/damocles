package sectors

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	ffiproof "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var _ api.SectorTracker = (*Tracker)(nil)

func NewTracker(indexer api.SectorIndexer) (*Tracker, error) {
	return &Tracker{
		indexer: indexer,
	}, nil
}

type Tracker struct {
	indexer api.SectorIndexer
}

func (t *Tracker) Provable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, strict bool) (map[abi.SectorNumber]string, error) {
	results := make([]string, len(sectors))
	var wg sync.WaitGroup
	wg.Add(len(sectors))

	ssize, err := pp.SectorSize()
	if err != nil {
		return nil, err
	}

	for ti := range sectors {
		go func(i int) {
			var reason string
			defer func() {
				if reason != "" {
					results[i] = reason
				}

				wg.Done()
			}()

			sid := sectors[i].ID
			objins, err := t.getObjInstanceForSector(ctx, sid)
			if err != nil {
				reason = fmt.Sprintf("get objstore instance for %s: %s", util.FormatSectorID(sid), err)
				return
			}

			// todo: for snapdeals
			subSealed := util.SectorPath(util.SectorPathTypeSealed, sid)
			_, err = objins.Stat(ctx, subSealed)
			if err != nil {
				reason = fmt.Sprintf("get stat info for %s: %s", util.FormatSectorID(sid), err)
				return
			}

			if !strict {
				return
			}

			subCache := util.SectorPath(util.SectorPathTypeCache, sid)
			toCheck := map[string]int64{
				filepath.Join(subCache, "p_aux"): 0,
			}
			addCachePathsForSectorSize(toCheck, subCache, ssize)

			for p, sz := range toCheck {
				st, err := objins.Stat(ctx, p)
				if err != nil {
					reason = fmt.Sprintf("get %s stat info for %s: %s", p, util.FormatSectorID(sid), err)
					return
				}

				if sz != 0 {
					if st.Size != int64(ssize)*sz {
						reason = fmt.Sprintf("%s is wrong size (got %d, expect %d)", p, st.Size, int64(ssize)*sz)
						return
					}
				}
			}

			// TODO more strictly checks

			return

		}(ti)
	}

	wg.Wait()

	bad := map[abi.SectorNumber]string{}
	for ri := range results {
		if results[ri] != "" {
			bad[sectors[ri].ID.Number] = results[ri]
		}
	}

	return bad, nil
}

func (t *Tracker) PubToPrivate(ctx context.Context, aid abi.ActorID, sectorInfo []builtin.ExtendedSectorInfo) (api.SortedPrivateSectorInfo, error) {
	out := make([]api.PrivateSectorInfo, 0, len(sectorInfo))
	for _, sector := range sectorInfo {
		sid := storage.SectorRef{
			ID:        abi.SectorID{Miner: aid, Number: sector.SectorNumber},
			ProofType: sector.SealProof,
		}

		postProofType, err := sid.ProofType.RegisteredWindowPoStProof()
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("acquiring registered PoSt proof from sector info %+v: %w", sector, err)
		}

		objins, err := t.getObjInstanceForSector(ctx, sid.ID)
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

		ffiInfo := ffiproof.SectorInfo{
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

func (t *Tracker) getObjInstanceForSector(ctx context.Context, sid abi.SectorID) (objstore.Store, error) {
	insname, has, err := t.indexer.Find(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find objstore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("objstore instance not found")
	}

	instance, err := t.indexer.StoreMgr().GetInstance(ctx, insname)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s: %w", insname, err)
	}

	return instance, nil
}

func addCachePathsForSectorSize(chk map[string]int64, cacheDir string, ssize abi.SectorSize) {
	switch ssize {
	case 2 << 10:
		fallthrough
	case 8 << 20:
		fallthrough
	case 512 << 20:
		chk[filepath.Join(cacheDir, "sc-02-data-tree-r-last.dat")] = 0
	case 32 << 30:
		for i := 0; i < 8; i++ {
			chk[filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i))] = 0
		}
	case 64 << 30:
		for i := 0; i < 16; i++ {
			chk[filepath.Join(cacheDir, fmt.Sprintf("sc-02-data-tree-r-last-%d.dat", i))] = 0
		}
	default:
		log.Warnf("not checking cache files of %s sectors for faults", ssize)
	}
}
