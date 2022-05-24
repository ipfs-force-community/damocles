package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

var _ core.DealManager = (*nullDeal)(nil)

func NewDealManager() core.DealManager {
	return &nullDeal{}
}

type nullDeal struct {
}

func (*nullDeal) Acquire(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec, wjob core.SectorWorkerJob) (core.Deals, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal core.AcquireDealsSpec: %w", err)
	}

	log.Info(string(b))
	return nil, nil
}

func (*nullDeal) Release(context.Context, abi.SectorID, core.Deals) error {
	return nil
}
