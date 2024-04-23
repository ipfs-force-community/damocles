package sectors

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("sectors")

var _ core.SectorManager = (*Manager)(nil)

var errMinerDisabled = fmt.Errorf("miner disblaed")

func NewManager(
	scfg *modules.SafeConfig,
	mapi core.MinerAPI,
	numAlloc core.SectorNumberAllocator,
) (*Manager, error) {
	mgr := &Manager{
		msel:     newMinerSelector(scfg, mapi),
		numAlloc: numAlloc,
	}

	return mgr, nil
}

type Manager struct {
	msel     *minerSelector
	numAlloc core.SectorNumberAllocator
}

func (m *Manager) Allocate(
	ctx context.Context,
	spec core.AllocateSectorSpec,
	count uint32,
) ([]*core.AllocatedSector, error) {
	allowedMiners := spec.AllowedMiners
	allowedProofs := spec.AllowedProofTypes
	if count == 0 {
		return nil, fmt.Errorf("at least one sector is allocated each call")
	}

	candidates := m.msel.candidates(ctx, allowedMiners, allowedProofs, func(mcfg modules.MinerConfig) bool {
		return mcfg.Sector.Enabled
	}, "sealing")

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
				if !ok && selected.cfg.Verbose {
					log.Warnw("max number exceeded", "max", max, "miner", selected.info.ID)
				}
				return ok
			}
		}

		minNumber := selected.cfg.InitNumber
		if selected.cfg.MinNumber != nil {
			minNumber = *selected.cfg.MinNumber
		}

		next, available, err := m.numAlloc.NextN(ctx, selected.info.ID, count, minNumber, check)
		if err != nil {
			return nil, err
		}

		if !available {
			candidates[candidateCount-1], candidates[selectIdx] = candidates[selectIdx], candidates[candidateCount-1]
			candidates = candidates[:candidateCount-1]
			continue
		}

		var (
			i  uint32
			id = next - uint64(count) + 1
		)
		allocatedSectors := make([]*core.AllocatedSector, count)
		for i < count {
			allocatedSectors[i] = &core.AllocatedSector{
				ID: abi.SectorID{
					Miner:  selected.info.ID,
					Number: abi.SectorNumber(id),
				},
				ProofType: selected.info.SealProofType,
			}
			i++
			id++
		}
		return allocatedSectors, nil
	}
}
