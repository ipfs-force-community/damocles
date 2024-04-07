package pledge

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/venus/venus-shared/actors/builtin/verifreg"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

type AllocationAPI interface {
	StateGetAllocationForPendingDeal(ctx context.Context, dealID abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error) //perm:read
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationID verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
}

func GetAllocation(ctx context.Context, aapi AllocationAPI, tsk types.TipSetKey, piece core.SectorPiece) (*verifregtypes.Allocation, error) {
	if !piece.HasDealInfo() {
		return nil, nil
	}
	if piece.IsBuiltinMarket() {
		return aapi.StateGetAllocationForPendingDeal(ctx, piece.DealID(), tsk)
	}

	client := piece.Client()
	all, err := aapi.StateGetAllocation(ctx, client, piece.AllocationID(), tsk)

	if err != nil {
		return nil, err
	}

	if all == nil {
		return nil, nil
	}

	onChainClient, err := address.NewIDAddress(uint64(all.Client))
	if err != nil {
		return nil, err
	}
	if onChainClient != client {
		return nil, fmt.Errorf("allocation client mismatch: %s(chain) != %s", onChainClient, client)
	}

	return all, nil
}
