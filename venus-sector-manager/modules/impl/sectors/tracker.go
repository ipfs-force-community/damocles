package sectors

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

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

func (t *Tracker) SinglePubToPrivateInfo(ctx context.Context, mid abi.ActorID, sector builtin.ExtendedSectorInfo, locator api.SectorLocator) (api.PrivateSectorInfo, error) {
	sref := api.SectorRef{
		ID:        abi.SectorID{Miner: mid, Number: sector.SectorNumber},
		ProofType: sector.SealProof,
	}

	return t.SinglePrivateInfo(ctx, sref, sector.SectorKey != nil, locator)
}

func (t *Tracker) SinglePrivateInfo(ctx context.Context, sref api.SectorRef, upgrade bool, locator api.SectorLocator) (api.PrivateSectorInfo, error) {
	objins, err := t.getObjInstanceForSector(ctx, sref.ID, locator, upgrade)
	if err != nil {
		return api.PrivateSectorInfo{}, fmt.Errorf("get location for %s: %w", util.FormatSectorID(sref.ID), err)
	}

	var cache string
	var sealed string
	if upgrade {
		cache = util.SectorPath(util.SectorPathTypeUpdateCache, sref.ID)
		sealed = util.SectorPath(util.SectorPathTypeUpdate, sref.ID)
	} else {
		cache = util.SectorPath(util.SectorPathTypeCache, sref.ID)
		sealed = util.SectorPath(util.SectorPathTypeSealed, sref.ID)
	}

	return api.PrivateSectorInfo{
		AccessInstance:   objins.Instance(ctx),
		CacheDirURI:      cache,
		CacheDirPath:     objins.FullPath(ctx, cache),
		SealedSectorURI:  sealed,
		SealedSectorPath: objins.FullPath(ctx, sealed),
	}, nil
}

func (t *Tracker) SingleProvable(ctx context.Context, sref api.SectorRef, upgrade bool, locator api.SectorLocator, strict bool) error {
	ssize, err := sref.ProofType.SectorSize()
	if err != nil {
		return fmt.Errorf("get sector size: %w", err)
	}

	privateInfo, err := t.SinglePrivateInfo(ctx, sref, upgrade, locator)
	if err != nil {
		return fmt.Errorf("get private info: %w", err)
	}

	objins, err := t.indexer.StoreMgr().GetInstance(ctx, privateInfo.AccessInstance)
	if err != nil {
		return fmt.Errorf("get obj instance named %s: %w", privateInfo.AccessInstance, err)
	}

	toCheck := map[string]int64{
		privateInfo.SealedSectorURI:                     1,
		filepath.Join(privateInfo.CacheDirURI, "p_aux"): 0,
	}

	addCachePathsForSectorSize(toCheck, privateInfo.CacheDirURI, ssize)

	for p, sz := range toCheck {
		st, err := objins.Stat(ctx, p)
		if err != nil {
			return fmt.Errorf("stat object: %w", err)
		}

		if sz != 0 {
			if st.Size != int64(ssize)*sz {
				return fmt.Errorf("%s with wrong size (got %d, expect %d)", p, st.Size, int64(ssize)*sz)
			}
		}
	}

	if !strict {
		return nil
	}

	// TODO strict check, winning post?
	return nil
}

func (t *Tracker) Provable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
	results := make([]string, len(sectors))
	var wg sync.WaitGroup
	wg.Add(len(sectors))

	for ti := range sectors {
		go func(i int) {
			defer wg.Done()

			sector := sectors[i]

			sref := api.SectorRef{
				ID:        abi.SectorID{Miner: mid, Number: sector.SectorNumber},
				ProofType: sector.SealProof,
			}
			err := t.SingleProvable(ctx, sref, sector.SectorKey != nil, nil, strict)
			if err == nil {
				return
			}

			results[i] = err.Error()

		}(ti)
	}

	wg.Wait()

	bad := map[abi.SectorNumber]string{}
	for ri := range results {
		if results[ri] != "" {
			bad[sectors[ri].SectorNumber] = results[ri]
		}
	}

	return bad, nil
}

func (t *Tracker) PubToPrivate(ctx context.Context, aid abi.ActorID, sectorInfo []builtin.ExtendedSectorInfo, typer api.SectorPoStTyper) ([]api.FFIPrivateSectorInfo, error) {
	if len(sectorInfo) == 0 {
		return []api.FFIPrivateSectorInfo{}, nil
	}

	out := make([]api.FFIPrivateSectorInfo, 0, len(sectorInfo))
	proofType, err := typer(sectorInfo[0].SealProof)
	if err != nil {
		return nil, fmt.Errorf("get PoSt proof type: %w", err)
	}

	for _, sector := range sectorInfo {
		priv, err := t.SinglePubToPrivateInfo(ctx, aid, sector, nil)
		if err != nil {
			return nil, fmt.Errorf("construct private info for %d: %w", sector.SectorNumber, err)
		}
		out = append(out, priv.ToFFI(util.SectorExtendedToNormal(sector), proofType))
	}

	return out, nil
}

func (t *Tracker) getObjInstanceForSector(ctx context.Context, sid abi.SectorID, locator api.SectorLocator, upgrade bool) (objstore.Store, error) {
	if locator == nil {
		if upgrade {
			locator = t.indexer.Upgrade().Find
		} else {
			locator = t.indexer.Normal().Find
		}
	}

	insname, has, err := locator(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find objstore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("object not found")
	}

	instance, err := t.indexer.StoreMgr().GetInstance(ctx, insname)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s: %w", insname, err)
	}

	return instance, nil
}

func addCachePathsForSectorSize(chk map[string]int64, cacheDir string, ssize abi.SectorSize) {
	files := util.CachedFilesForSectorSize(cacheDir, ssize)
	for fi := range files {
		chk[files[fi]] = 0
	}
}
