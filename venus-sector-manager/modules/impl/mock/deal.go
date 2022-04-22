package mock

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

var _ core.DealManager = (*nullDeal)(nil)

func NewDealManager() core.DealManager {
	return &nullDeal{}
}

type nullDeal struct {
}

func (*nullDeal) Acquire(context.Context, abi.SectorID, core.AcquireDealsSpec, core.SectorWorkerJob) (core.Deals, error) {
	return nil, nil
}

func (*nullDeal) Release(context.Context, abi.SectorID, core.Deals) error {
	return nil
}
