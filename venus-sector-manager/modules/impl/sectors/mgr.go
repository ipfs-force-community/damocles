package sectors

import (
	"context"
	"math/rand"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
)

var _ api.SectorManager = (*Manager)(nil)

func NewManager(
	cfg *modules.Config,
	locker confmgr.RLocker,
	mapi api.MinerInfoAPI,
	numAlloc api.SectorNumberAllocator,
) (*Manager, error) {
	mgr := &Manager{
		info:     mapi,
		numAlloc: numAlloc,
	}

	mgr.cfg.Config = cfg
	mgr.cfg.RLocker = locker
	return mgr, nil
}

type Manager struct {
	cfg struct {
		*modules.Config
		confmgr.RLocker
	}

	info     api.MinerInfoAPI
	numAlloc api.SectorNumberAllocator
}

func (m *Manager) Allocate(ctx context.Context, allowedMiners []abi.ActorID, allowedProofs []abi.RegisteredSealProof) (*api.AllocatedSector, error) {
	m.cfg.Lock()
	mids := m.cfg.SectorManager.Miners
	m.cfg.Unlock()

	if len(mids) == 0 {
		return nil, nil
	}

	midCnt := len(mids)

	var wg sync.WaitGroup
	infos := make([]*api.MinerInfo, midCnt)
	errs := make([]error, midCnt)

	wg.Add(midCnt)
	for i := range mids {
		go func(mi int) {
			defer wg.Done()
			mid := mids[mi]

			minfo, err := m.info.Get(ctx, mid)
			if err == nil {
				infos[mi] = minfo
			} else {
				errs[mi] = err
			}
		}(i)
	}

	wg.Wait()

	// TODO: trace errors
	last := len(mids)
	i := 0
	for i < last {
		minfo := infos[i]
		ok := minfo != nil
		if ok && len(allowedMiners) > 0 {
			should := false
			for ai := range allowedMiners {
				if minfo.ID == allowedMiners[ai] {
					should = true
					break
				}
			}

			ok = should
		}

		if ok && len(allowedProofs) > 0 {
			should := false
			for ai := range allowedProofs {
				if minfo.SealProofType == allowedProofs[ai] {
					should = true
					break
				}
			}

			ok = should
		}

		if !ok {
			infos[i], infos[last-1] = infos[last-1], infos[i]
			last--
			continue
		}

		i++
	}

	candidates := infos[:last]
	if len(candidates) == 0 {
		return nil, nil
	}

	selected := candidates[rand.Intn(len(candidates))]
	next, err := m.numAlloc.Next(ctx, selected.ID)
	if err != nil {
		return nil, err
	}

	return &api.AllocatedSector{
		ID: abi.SectorID{
			Miner:  selected.ID,
			Number: abi.SectorNumber(next),
		},
		ProofType: selected.SealProofType,
	}, nil
}
