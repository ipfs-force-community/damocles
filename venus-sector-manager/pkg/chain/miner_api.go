package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

var _ core.MinerInfoAPI = (*MinerInfoAPI)(nil)

func NewMinerInfoAPI(capi API) *MinerInfoAPI {
	return &MinerInfoAPI{
		chain: capi,
		cache: map[abi.ActorID]*core.MinerInfo{},
	}
}

type MinerInfoAPI struct {
	// TODO: miner info cache
	chain   API
	cacheMu sync.RWMutex
	cache   map[abi.ActorID]*core.MinerInfo
}

func (m *MinerInfoAPI) Get(ctx context.Context, mid abi.ActorID) (*core.MinerInfo, error) {
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

	dlinfo, err := m.chain.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("get proving deadline info: %w", err)
	}

	sealProof, err := miner.SealProofTypeFromSectorSize(minfo.SectorSize, constants.NewestNetworkVersion)
	if err != nil {
		return nil, err
	}

	mi = &core.MinerInfo{
		ID:                  mid,
		Addr:                maddr,
		SectorSize:          minfo.SectorSize,
		WindowPoStProofType: minfo.WindowPoStProofType,
		SealProofType:       sealProof,
		Deadline:            *dlinfo,
	}

	m.cacheMu.Lock()
	m.cache[mid] = mi
	m.cacheMu.Unlock()

	return mi, nil
}
