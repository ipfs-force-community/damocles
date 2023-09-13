package sectors

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/filestore"
)

var _ core.SectorTracker = (*Tracker)(nil)

type sectorStoreInstances struct {
	info       core.SectorAccessStores
	sealedFile filestore.Ext
	cacheDir   filestore.Ext
}

func NewTracker(indexer core.SectorIndexer) (*Tracker, error) {
	return &Tracker{
		indexer: indexer,
	}, nil
}

type Tracker struct {
	indexer core.SectorIndexer
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

	var sealedPathType filestore.PathType
	var cachePathType filestore.PathType
	if upgrade {
		sealedPathType = filestore.PathTypeUpdate
		cachePathType = filestore.PathTypeUpdateCache
	} else {
		sealedPathType = filestore.PathTypeSealed
		cachePathType = filestore.PathTypeCache
	}
	cacheDirFullPath, cacheDirSubPath, err := objins.cacheDir.FullPath(ctx, cachePathType, &sref.ID, nil)
	if err != nil {
		return nil, core.PrivateSectorInfo{}, fmt.Errorf("get cache FullPath(%s): %w", sref.ID, err)
	}
	sealedFullPath, sealedSubPath, err := objins.sealedFile.FullPath(ctx, sealedPathType, &sref.ID, nil)
	if err != nil {
		return nil, core.PrivateSectorInfo{}, fmt.Errorf("get sealed FullPath(%s): %w", sealedFullPath, err)
	}
	return objins, core.PrivateSectorInfo{
		Accesses:         objins.info,
		CacheDirFullPath: cacheDirFullPath,
		SealedFullPath:   sealedFullPath,
		CacheDirSubPath:  cacheDirSubPath,
		SealedSubPath:    sealedSubPath,
	}, nil
}

func (t *Tracker) SinglePrivateInfo(ctx context.Context, sref core.SectorRef, upgrade bool, locator core.SectorLocator) (core.PrivateSectorInfo, error) {
	_, privateInfo, err := t.getPrivateInfo(ctx, sref, upgrade, locator)
	if err != nil {
		return core.PrivateSectorInfo{}, fmt.Errorf("get private info: %w", err)
	}

	return privateInfo, nil
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
		return nil, fmt.Errorf("find filestore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("object not found")
	}

	instances := sectorStoreInstances{
		info: access,
	}
	instances.sealedFile, err = t.indexer.StoreMgr().GetInstance(ctx, access.SealedFile)
	if err != nil {
		return nil, fmt.Errorf("get filestore instance %s for sealed file: %w", access.SealedFile, err)
	}

	instances.cacheDir, err = t.indexer.StoreMgr().GetInstance(ctx, access.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("get filestore instance %s for cache dir: %w", access.CacheDir, err)
	}

	return &instances, nil
}

func addCachePathsForSectorSize(chk map[string]int64, cacheDir string, ssize abi.SectorSize) {
	files := util.CachedFilesForSectorSize(cacheDir, ssize)
	for fi := range files {
		chk[files[fi]] = 0
	}
}
