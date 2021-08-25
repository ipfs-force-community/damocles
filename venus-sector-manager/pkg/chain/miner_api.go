package chain

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/dtynn/venus-cluster/venus-sector-manager/api"
)

var _ api.MinerInfoAPI = (*MinerInfoAPI)(nil)

func NewMinerInfoAPI(capi API) *MinerInfoAPI {
	return &MinerInfoAPI{
		chain: capi,
		cache: map[abi.ActorID]*api.MinerInfo{},
	}
}

type MinerInfoAPI struct {
	// TODO: miner info cache
	chain   API
	cacheMu sync.RWMutex
	cache   map[abi.ActorID]*api.MinerInfo
}

func (m *MinerInfoAPI) Get(ctx context.Context, mid abi.ActorID) (*api.MinerInfo, error) {
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

	sealProof, err := miner.SealProofTypeFromSectorSize(minfo.SectorSize, constants.NewestNetworkVersion)
	if err != nil {
		return nil, err
	}

	mi = &api.MinerInfo{
		ID:                  mid,
		SectorSize:          minfo.SectorSize,
		WindowPoStProofType: minfo.WindowPoStProofType,
		SealProofType:       sealProof,
	}

	m.cacheMu.Lock()
	m.cache[mid] = mi
	m.cacheMu.Unlock()

	return mi, nil
}
