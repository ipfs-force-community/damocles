package chain

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
)

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

func (m *MinerAPI) GetInfo(ctx context.Context, mid abi.ActorID) (*core.MinerInfo, error) {
	m.cacheMu.RLock()
	mi, ok := m.cache[mid]
	m.cacheMu.RUnlock()
	if ok {
		return mi, nil
	}

	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return nil, err
	}

	minfo, err := m.chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	sealProof, err := miner.SealProofTypeFromSectorSize(minfo.SectorSize, constants.TestNetworkVersion)
	if err != nil {
		return nil, err
	}

	mi = &core.MinerInfo{
		ID:                  mid,
		Addr:                maddr,
		SectorSize:          minfo.SectorSize,
		WindowPoStProofType: minfo.WindowPoStProofType,
		SealProofType:       sealProof,
	}

	m.cacheMu.Lock()
	m.cache[mid] = mi
	m.cacheMu.Unlock()

	return mi, nil
}

func (m *MinerAPI) GetMinerConfig(ctx context.Context, mid abi.ActorID) (*modules.MinerConfig, error) {
	config, err := m.safeConfig.MinerConfig(mid)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
