package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var minerAPILog = logging.New("miner-api")

var _ core.MinerAPI = (*MinerAPI)(nil)

func NewMinerAPI(capi API, safeConfig *modules.SafeConfig) *MinerAPI {
	return &MinerAPI{
		chain:      capi,
		cache:      map[abi.ActorID]*core.MinerInfo{},
		safeConfig: safeConfig,
	}
}

type MinerAPI struct {
	chain      API
	cacheMu    sync.RWMutex
	cache      map[abi.ActorID]*core.MinerInfo
	safeConfig *modules.SafeConfig
}

func (m *MinerAPI) PrefetchCache(ctx context.Context) {
	m.safeConfig.Lock()
	miners := m.safeConfig.Miners
	m.safeConfig.Unlock()

	if len(miners) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(miners))

	for i := range miners {
		go func(mi int) {
			defer wg.Done()
			mid := miners[mi].Actor

			mlog := minerAPILog.With("miner", mid)
			info, err := m.GetInfo(ctx, mid)
			if err == nil {
				mlog.Infof("miner info pre-fetched: %#v", info)
			} else {
				mlog.Warnf("miner info pre-fetch failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

func (m *MinerAPI) GetInfo(ctx context.Context, mid abi.ActorID) (mi *core.MinerInfo, err error) {
	m.cacheMu.RLock()
	mi, ok := m.cache[mid]
	m.cacheMu.RUnlock()
	if ok {
		// return mi, nil

	} else {

		maddr, err := address.NewIDAddress(uint64(mid))
		if err != nil {
			return nil, err
		}

		minfo, err := m.chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		mi = &core.MinerInfo{
			ID:                  mid,
			Addr:                maddr,
			SectorSize:          minfo.SectorSize,
			WindowPoStProofType: minfo.WindowPoStProofType,
		}

		m.cacheMu.Lock()
		m.cache[mid] = mi
		m.cacheMu.Unlock()
	}

	useSyntheticPoRep := false
	m.safeConfig.Lock()
	minerCfgs := m.safeConfig.Config.Miners
	for _, minerCfg := range minerCfgs {
		if minerCfg.Actor == mid {
			useSyntheticPoRep = minerCfg.Sealing.UseSyntheticPoRep
			break
		}
	}
	m.safeConfig.Unlock()

	nv, err := m.chain.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("get network version: %w", err)
	}

	sealProof, err := miner.SealProofTypeFromSectorSize(mi.SectorSize, nv, useSyntheticPoRep)
	if err != nil {
		return nil, err
	}
	mi.SealProofType = sealProof

	return mi, nil
}

func (m *MinerAPI) GetMinerConfig(_ context.Context, mid abi.ActorID) (*modules.MinerConfig, error) {
	config, err := m.safeConfig.MinerConfig(mid)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
