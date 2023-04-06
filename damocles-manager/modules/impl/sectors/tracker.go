package sectors

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	chainAPI "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
)

var _ core.SectorTracker = (*Tracker)(nil)

type sectorStoreInstances struct {
	info       core.SectorAccessStores
	sealedFile objstore.Store
	cacheDir   objstore.Store
}

func NewTracker(indexer core.SectorIndexer, state core.SectorStateManager, prover core.Prover, capi chainAPI.API, stCfg modules.ProvingConfig) (*Tracker, error) {
	return &Tracker{
		indexer: indexer,
		state:   state,
		prover:  prover,
		capi:    capi,

		parallelCheckLimit:    stCfg.ParallelCheckLimit,
		singleCheckTimeout:    time.Duration(stCfg.SingleCheckTimeout),
		partitionCheckTimeout: time.Duration(stCfg.PartitionCheckTimeout),
	}, nil
}

type Tracker struct {
	indexer core.SectorIndexer
	state   core.SectorStateManager
	prover  core.Prover
	capi    chainAPI.API

	parallelCheckLimit    int
	singleCheckTimeout    time.Duration
	partitionCheckTimeout time.Duration
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

func (t *Tracker) SingleProvable(ctx context.Context, sref core.SectorRef, upgrade bool, locator core.SectorLocator, strict, stateCheck bool) error {
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

	proofType, err := sref.ProofType.RegisteredWindowPoStProof()
	if err != nil {
		return err
	}

	addr, err := address.NewIDAddress(uint64(sref.ID.Miner))
	if err != nil {
		return err
	}
	sinfo, err := t.capi.StateSectorGetInfo(ctx, addr, sref.ID.Number, types.EmptyTSK)
	if err != nil {
		return err
	}

	if stateCheck {
		// local and chain consistency check
		ss, err := t.state.Load(ctx, sref.ID, core.WorkerOffline)
		if err != nil {
			return fmt.Errorf("not exist in Offline, maybe in Online: %w", err)
		}
		// for snap: onChain.SealedCID == local.UpgradedInfo.SealedCID, onChain.SectorKeyCID == ss.Pre.CommR, for other(CC/DC): onChain.SealedCID == onChain.SealedCID
		if !upgrade {
			if !ss.Pre.CommR.Equals(sinfo.SealedCID) {
				return fmt.Errorf("the SealedCID on the local and the chain is inconsistent")
			}
		} else {
			if !sinfo.SectorKeyCID.Equals(ss.Pre.CommR) {
				return fmt.Errorf("the SectorKeyCID on the local and the chain is inconsistent")
			}

			// 从 lotus 导入的扇区 UpgradedInfo 是空值,见代码: damocles-manager/cmd/damocles-manager/internal/util_sealer_sectors.go#L1735
			if ss.UpgradedInfo.SealedCID != cid.Undef && !sinfo.SealedCID.Equals(ss.UpgradedInfo.SealedCID) {
				return fmt.Errorf("the SealedCID on the local and the chain is inconsistent")
			}
		}
	}

	replica := privateInfo.ToFFI(core.SectorInfo{
		SealProof:    sref.ProofType,
		SectorNumber: sref.ID.Number,
		SealedCID:    sinfo.SealedCID,
	}, proofType)

	scCtx := ctx
	if t.singleCheckTimeout > 0 {
		var scCancel context.CancelFunc
		scCtx, scCancel = context.WithTimeout(ctx, t.singleCheckTimeout)
		defer scCancel()
	}

	// use randUint64 % nodeNums as challenge, notice nodeNums = ssize / 32B
	_, err = t.prover.GenerateSingleVanillaProof(scCtx, replica, []uint64{rand.Uint64() % (uint64(ssize) / 32)})

	if err != nil {
		return fmt.Errorf("generate vanilla proof of %s failed: %w", sref.ID, err)
	}

	return nil
}

func (t *Tracker) Provable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict, stateCheck bool) (map[abi.SectorNumber]string, error) {
	limit := t.parallelCheckLimit
	if limit <= 0 {
		limit = len(sectors)
	}
	throttle := make(chan struct{}, limit)

	if t.partitionCheckTimeout > 0 {
		var pcCancel context.CancelFunc
		ctx, pcCancel = context.WithTimeout(ctx, t.partitionCheckTimeout)
		defer pcCancel()
	}

	results := make([]string, len(sectors))
	var wg sync.WaitGroup
	wg.Add(len(sectors))

	for ti := range sectors {
		select {
		case throttle <- struct{}{}:
		case <-ctx.Done():
			// After the overtime, walk through the cycle and do not turn on the thread check.
			results[ti] = fmt.Sprintf("waiting for check worker: %s", ctx.Err())
			wg.Done()
			continue
		}

		go func(i int) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			sector := sectors[i]

			sref := core.SectorRef{
				ID:        abi.SectorID{Miner: mid, Number: sector.SectorNumber},
				ProofType: sector.SealProof,
			}
			err := t.SingleProvable(ctx, sref, sector.SectorKey != nil, nil, strict, stateCheck)
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

func (t *Tracker) PubToPrivate(ctx context.Context, aid abi.ActorID, sectorInfo []builtin.ExtendedSectorInfo, typer core.SectorPoStTyper) ([]core.FFIPrivateSectorInfo, error) {
	if len(sectorInfo) == 0 {
		return []core.FFIPrivateSectorInfo{}, nil
	}

	out := make([]core.FFIPrivateSectorInfo, 0, len(sectorInfo))
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
