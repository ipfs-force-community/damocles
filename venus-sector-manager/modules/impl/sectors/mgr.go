package sectors

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("sectors")

var _ api.SectorManager = (*Manager)(nil)

var errMinerDisabled = fmt.Errorf("miner disblaed")

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

type minerCandidate struct {
	info *api.MinerInfo
	cfg  *modules.MinerSectorConfig
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
	miners := m.cfg.Miners
	m.cfg.Unlock()

	if len(miners) == 0 {
		return nil, nil
	}

	midCnt := len(miners)

	var wg sync.WaitGroup
	infos := make([]*minerCandidate, midCnt)
	errs := make([]error, midCnt)

	wg.Add(midCnt)
	for i := range miners {
		go func(mi int) {
			defer wg.Done()

			if !miners[mi].Sector.Enabled {
				log.Warnw("sector allocator disabled", "miner", miners[mi].Actor)
				errs[mi] = errMinerDisabled
				return
			}

			mid := miners[mi].Actor

			minfo, err := m.info.Get(ctx, mid)
			if err == nil {
				infos[mi] = &minerCandidate{
					info: minfo,
					cfg:  &miners[mi].Sector,
				}
			} else {
				errs[mi] = err
			}
		}(i)
	}

	wg.Wait()

	// TODO: trace errors
	last := len(miners)
	i := 0
	for i < last {
		minfo := infos[i]
		ok := minfo != nil
		if ok && len(allowedMiners) > 0 {
			should := false
			for ai := range allowedMiners {
				if minfo.info.ID == allowedMiners[ai] {
					should = true
					break
				}
			}

			ok = should
		}

		if ok && len(allowedProofs) > 0 {
			should := false
			for ai := range allowedProofs {
				if minfo.info.SealProofType == allowedProofs[ai] {
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
	for {
		candidateCount := len(candidates)
		if candidateCount == 0 {
			return nil, nil
		}

		selectIdx := rand.Intn(candidateCount)
		selected := candidates[selectIdx]

		var check func(uint64) bool
		if selected.cfg.MaxNumber == nil {
			check = func(uint64) bool { return true }
		} else {
			max := *selected.cfg.MaxNumber
			check = func(next uint64) bool {
				ok := next <= max
				if !ok {
					log.Warnw("max number exceeded", "max", max, "miner", selected.info.ID)
				}
				return ok
			}
		}

		next, available, err := m.numAlloc.Next(ctx, selected.info.ID, selected.cfg.InitNumber, check)
		if err != nil {
			return nil, err
		}

		if available {
			return &api.AllocatedSector{
				ID: abi.SectorID{
					Miner:  selected.info.ID,
					Number: abi.SectorNumber(next),
				},
				ProofType: selected.info.SealProofType,
			}, nil
		}

		candidates[candidateCount-1], candidates[selectIdx] = candidates[selectIdx], candidates[candidateCount-1]
		candidates = candidates[:candidateCount-1]
	}
}
