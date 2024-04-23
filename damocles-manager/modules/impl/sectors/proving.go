package sectors

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
	"github.com/ipfs/go-cid"
)

func NewProving(
	sectorTracker core.SectorTracker,
	state core.SectorStateManager,
	storeMgr objstore.Manager,
	prover core.Prover,
	capi chainapi.API,
	stCfg modules.ProvingConfig,
) (core.SectorProving, error) {
	return &Proving{
		SectorTracker: sectorTracker,
		state:         state,
		storeMgr:      storeMgr,
		prover:        prover,
		capi:          capi,

		parallelCheckLimit:    stCfg.ParallelCheckLimit,
		singleCheckTimeout:    time.Duration(stCfg.SingleCheckTimeout),
		partitionCheckTimeout: time.Duration(stCfg.PartitionCheckTimeout),
	}, nil
}

type Proving struct {
	core.SectorTracker
	state    core.SectorStateManager
	storeMgr objstore.Manager
	prover   core.Prover
	capi     chainapi.API

	parallelCheckLimit    int
	singleCheckTimeout    time.Duration
	partitionCheckTimeout time.Duration
}

func (p *Proving) SingleProvable(
	ctx context.Context,
	postProofType abi.RegisteredPoStProof,
	sref core.SectorRef,
	upgrade bool,
	locator core.SectorLocator,
	strict, stateCheck bool,
) error {
	ssize, err := sref.ProofType.SectorSize()
	if err != nil {
		return fmt.Errorf("get sector size: %w", err)
	}

	privateInfo, err := p.SectorTracker.SinglePrivateInfo(ctx, sref, upgrade, locator)
	if err != nil {
		return fmt.Errorf("get private info: %w", err)
	}
	sealedFileIns, err := p.storeMgr.GetInstance(ctx, privateInfo.Accesses.SealedFile)
	if err != nil {
		return fmt.Errorf("get objstore instance %s for sealed file: %w", privateInfo.Accesses.SealedFile, err)
	}

	cacheDirIns, err := p.storeMgr.GetInstance(ctx, privateInfo.Accesses.CacheDir)
	if err != nil {
		return fmt.Errorf("get objstore instance %s for cache dir: %w", privateInfo.Accesses.CacheDir, err)
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
			store: sealedFileIns,
			targets: map[string]int64{
				privateInfo.SealedSectorURI: 1,
			},
		},
		{
			title:   "cache dir",
			store:   cacheDirIns,
			targets: targetsInCacheDir,
		},
	}

	if p.singleCheckTimeout > 0 {
		var scCancel context.CancelFunc
		ctx, scCancel = context.WithTimeout(ctx, p.singleCheckTimeout)
		defer scCancel()
	}

	for _, check := range checks {
		for p, sz := range check.targets {
			st, err := check.store.Stat(ctx, p)
			if err != nil {
				return fmt.Errorf("stat object %s for %s: %w", p, check.title, err)
			}

			if sz != 0 && strict {
				if st.Size != int64(ssize)*sz {
					return fmt.Errorf(
						"%s for %s with wrong size (got %d, expect %d)",
						p,
						check.title,
						st.Size,
						int64(ssize)*sz,
					)
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
	sinfo, err := p.capi.StateSectorGetInfo(ctx, addr, sref.ID.Number, types.EmptyTSK)
	if err != nil {
		return err
	}

	if stateCheck {
		// local and chain consistency check
		ss, err := p.state.Load(ctx, sref.ID, core.WorkerOffline)
		if err != nil {
			return fmt.Errorf("not exist in Offline, maybe in Online: %w", err)
		}
		//revive:disable-line:line-length-limit
		// for snap: onChain.SealedCID == local.UpgradedInfo.SealedCID, onChain.SectorKeyCID == ss.Pre.CommR, for other(CC/DC): onChain.SealedCID == onChain.SealedCID
		if !upgrade {
			if !ss.Pre.CommR.Equals(sinfo.SealedCID) {
				return fmt.Errorf("the SealedCID on the local and the chain is inconsistent")
			}
		} else {
			if !sinfo.SectorKeyCID.Equals(ss.Pre.CommR) {
				return fmt.Errorf("the SectorKeyCID on the local and the chain is inconsistent")
			}

			//revive:disable-line:line-length-limit
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
	}, postProofType)

	// use randUint64 % nodeNums as challenge, notice nodeNums = ssize / 32B
	_, err = p.prover.GenerateSingleVanillaProof(ctx, replica, []uint64{rand.Uint64() % (uint64(ssize) / 32)})

	if err != nil {
		return fmt.Errorf("generate vanilla proof of %s failed: %w", sref.ID, err)
	}

	return nil
}

func (p *Proving) Provable(
	ctx context.Context,
	mid abi.ActorID,
	postProofType abi.RegisteredPoStProof,
	sectors []builtin.ExtendedSectorInfo,
	strict, stateCheck bool,
) (map[abi.SectorNumber]string, error) {
	limit := p.parallelCheckLimit
	if limit <= 0 {
		limit = len(sectors)
	}
	throttle := make(chan struct{}, limit)

	if p.partitionCheckTimeout > 0 {
		var pcCancel context.CancelFunc
		ctx, pcCancel = context.WithTimeout(ctx, p.partitionCheckTimeout)
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
			err := p.SingleProvable(ctx, postProofType, sref, sector.SectorKey != nil, nil, strict, stateCheck)
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
