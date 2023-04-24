package sectors

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
)

type minerCandidate struct {
	info *core.MinerInfo
	cfg  *modules.MinerSectorConfig
}

func newMinerSelector(scfg *modules.SafeConfig, minerAPI core.MinerAPI) *minerSelector {
	return &minerSelector{
		scfg:     scfg,
		minerAPI: minerAPI,
	}
}

type minerSelector struct {
	scfg     *modules.SafeConfig
	minerAPI core.MinerAPI
}

func (m *minerSelector) candidates(ctx context.Context, allowedMiners []abi.ActorID, allowedProofs []abi.RegisteredSealProof, check func(mcfg modules.MinerConfig) bool, usage string) []*minerCandidate {
	m.scfg.Lock()
	miners := m.scfg.Miners
	m.scfg.Unlock()

	if len(miners) == 0 {
		return nil
	}

	midCnt := len(miners)

	var wg sync.WaitGroup
	infos := make([]*minerCandidate, midCnt)
	errs := make([]error, midCnt)

	// TODO: trace errors
	wg.Add(midCnt)
	for i := range miners {
		go func(mi int) {
			defer wg.Done()

			if !check(miners[mi]) {
				log.Warnw("sector allocator disabled", "miner", miners[mi].Actor, "usage", usage)
				errs[mi] = errMinerDisabled
				return
			}

			mid := miners[mi].Actor

			minfo, err := m.minerAPI.GetInfo(ctx, mid)
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
	return candidates
}
