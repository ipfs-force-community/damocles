package sectors

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	chainAPI "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var _ core.SectorTracker = (*Tracker)(nil)

type sectorStoreInstances struct {
	info       core.SectorAccessStores
	sealedFile objstore.Store
	cacheDir   objstore.Store
}

func NewTracker(indexer core.SectorIndexer, prover core.Prover, capi chainAPI.API) (*Tracker, error) {
	return &Tracker{
		indexer: indexer,
		prover:  prover,
		capi:    capi,
	}, nil
}

type Tracker struct {
	indexer core.SectorIndexer
	prover  core.Prover
	capi    chainAPI.API
}

func (t *Tracker) SinglePubToPrivateInfo(ctx context.Context, mid abi.ActorID, sector builtin.ExtendedSectorInfo, locator core.SectorLocator) (core.PrivateSectorInfo, error) {
	sref := core.SectorRef{
		ID:        abi.SectorID{Miner: mid, Number: sector.SectorNumber},
		ProofType: sector.SealProof,
	}

	return t.SinglePrivateInfo(ctx, sref, sector.SectorKey != nil, locator)
}

func (t *Tracker) getPrivateInfo(ctx context.Context, sref core.SectorRef, upgrade bool, locator core.SectorLocator) (*sectorStoreInstances, core.PrivateSectorInfo, error) {
	objins, err := t.getObjInstanceForSector(ctx, sref.ID, locator, upgrade)
	if err != nil {
		return nil, core.PrivateSectorInfo{}, fmt.Errorf("get location for %s: %w", util.FormatSectorID(sref.ID), err)
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

	return objins, core.PrivateSectorInfo{
		Accesses:         objins.info,
		CacheDirURI:      cache,
		CacheDirPath:     objins.cacheDir.FullPath(ctx, cache),
		SealedSectorURI:  sealed,
		SealedSectorPath: objins.sealedFile.FullPath(ctx, sealed),
	}, nil
}

func (t *Tracker) SinglePrivateInfo(ctx context.Context, sref core.SectorRef, upgrade bool, locator core.SectorLocator) (core.PrivateSectorInfo, error) {
	_, privateInfo, err := t.getPrivateInfo(ctx, sref, upgrade, locator)
	if err != nil {
		return core.PrivateSectorInfo{}, fmt.Errorf("get private info: %w", err)
	}

	return privateInfo, nil
}

func (t *Tracker) SingleProvable(ctx context.Context, postProofType abi.RegisteredPoStProof, sref core.SectorRef, upgrade bool, locator core.SectorLocator, strict bool) error {
	ssize, err := sref.ProofType.SectorSize()
	if err != nil {
		return fmt.Errorf("get sector size: %w", err)
	}

	instances, privateInfo, err := t.getPrivateInfo(ctx, sref, upgrade, locator)
	if err != nil {
		return fmt.Errorf("get private info: %w", err)
	}

	targetsInCacheDir := map[string]int64{}
	addCachePathsForSectorSize(targetsInCacheDir, privateInfo.CacheDirURI, ssize)

	checks := []struct {
		title   string
		store   objstore.Store
		targets map[string]int64
	}{
		{
			title: "sealed file",
			store: instances.sealedFile,
			targets: map[string]int64{
				privateInfo.SealedSectorURI: 1,
			},
		},
		{
			title:   "cache dir",
			store:   instances.cacheDir,
			targets: targetsInCacheDir,
		},
	}

	for _, check := range checks {
		for p, sz := range check.targets {
			st, err := check.store.Stat(ctx, p)
			if err != nil {
				return fmt.Errorf("stat object %s for %s: %w", p, check.title, err)
			}

			if sz != 0 {
				if st.Size != int64(ssize)*sz {
					return fmt.Errorf("%s for %s with wrong size (got %d, expect %d)", p, check.title, st.Size, int64(ssize)*sz)
				}
			}
		}
	}

	if !strict {
		return nil
	}

	addr, err := address.NewIDAddress(uint64(sref.ID.Miner))
	if err != nil {
		return err
	}
	sinfo, err := t.capi.StateSectorGetInfo(ctx, addr, sref.ID.Number, types.EmptyTSK)
	if err != nil {
		return err
	}
	replica := privateInfo.ToFFI(core.SectorInfo{
		SealProof:    sref.ProofType,
		SectorNumber: sref.ID.Number,
		SealedCID:    sinfo.SealedCID,
	}, postProofType)

	// use randUint64 % nodeNums as challenge, notice nodeNums = ssize / 32B
	_, err = t.prover.GenerateSingleVanillaProof(ctx, replica, []uint64{rand.Uint64() % (uint64(ssize) / 32)})

	if err != nil {
		return fmt.Errorf("generate vanilla proof of %s failed: %w", sref.ID, err)
	}

	return nil
}

func (t *Tracker) Provable(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
	results := make([]string, len(sectors))
	var wg sync.WaitGroup
	wg.Add(len(sectors))

	for ti := range sectors {
		go func(i int) {
			defer wg.Done()

			sector := sectors[i]

			sref := core.SectorRef{
				ID:        abi.SectorID{Miner: mid, Number: sector.SectorNumber},
				ProofType: sector.SealProof,
			}
			err := t.SingleProvable(ctx, postProofType, sref, sector.SectorKey != nil, nil, strict)
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

func (t *Tracker) PubToPrivate(ctx context.Context, aid abi.ActorID, postProofType abi.RegisteredPoStProof, sectorInfo []builtin.ExtendedSectorInfo) ([]core.FFIPrivateSectorInfo, error) {
	if len(sectorInfo) == 0 {
		return []core.FFIPrivateSectorInfo{}, nil
	}

	out := make([]core.FFIPrivateSectorInfo, 0, len(sectorInfo))
	for _, sector := range sectorInfo {
		priv, err := t.SinglePubToPrivateInfo(ctx, aid, sector, nil)
		if err != nil {
			return nil, fmt.Errorf("construct private info for %d: %w", sector.SectorNumber, err)
		}
		out = append(out, priv.ToFFI(util.SectorExtendedToNormal(sector), postProofType))
	}

	return out, nil
}

func (t *Tracker) getObjInstanceForSector(ctx context.Context, sid abi.SectorID, locator core.SectorLocator, upgrade bool) (*sectorStoreInstances, error) {
	if locator == nil {
		if upgrade {
			locator = t.indexer.Upgrade().Find
		} else {
			locator = t.indexer.Normal().Find
		}
	}

	access, has, err := locator(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find objstore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("object not found")
	}

	instances := sectorStoreInstances{
		info: access,
	}
	instances.sealedFile, err = t.indexer.StoreMgr().GetInstance(ctx, access.SealedFile)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s for sealed file: %w", access.SealedFile, err)
	}

	instances.cacheDir, err = t.indexer.StoreMgr().GetInstance(ctx, access.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s for cache dir: %w", access.CacheDir, err)
	}

	return &instances, nil
}

func addCachePathsForSectorSize(chk map[string]int64, cacheDir string, ssize abi.SectorSize) {
	files := util.CachedFilesForSectorSize(cacheDir, ssize)
	for fi := range files {
		chk[files[fi]] = 0
	}
}
