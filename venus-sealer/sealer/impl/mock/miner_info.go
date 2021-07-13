package mock

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

func NewMinerInfoAPI() api.MinerInfoAPI {
	return &minerInfo{}
}

var _ api.MinerInfoAPI = (*minerInfo)(nil)

type minerInfo struct {
}

func (m *minerInfo) Get(ctx context.Context, mid abi.ActorID) (*api.MinerInfo, error) {
	maddr, err := address.NewIDAddress(uint64(mid - 1))
	if err != nil {
		return nil, err
	}

	return &api.MinerInfo{
		ID:                  mid,
		Owner:               maddr,
		Worker:              maddr,
		WindowPoStProofType: abi.RegisteredPoStProof_StackedDrgWindow2KiBV1,
		SealProofType:       abi.RegisteredSealProof_StackedDrg2KiBV1_1,
	}, nil
}
