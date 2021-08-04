package chain

import (
	"context"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	"github.com/filecoin-project/venus/pkg/types"
)

var _ api.MinerInfoAPI = (*MinerInfoAPI)(nil)

func NewMinerInfoAPI(capi API) MinerInfoAPI {
	return MinerInfoAPI{
		chain: capi,
	}
}

type MinerInfoAPI struct {
	// TODO: miner info cache
	chain API
}

func (m *MinerInfoAPI) Get(ctx context.Context, mid abi.ActorID) (*api.MinerInfo, error) {
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

	return &api.MinerInfo{
		ID:                  mid,
		SectorSize:          minfo.SectorSize,
		WindowPoStProofType: minfo.WindowPoStProofType,
		SealProofType:       sealProof,
	}, nil
}
