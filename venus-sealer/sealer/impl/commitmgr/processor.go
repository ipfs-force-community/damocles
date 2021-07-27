package commitmgr

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type Processor interface {
	Process(ctx context.Context, sectors []api.SectorState, maddr address.Address) error

	Expire(ctx context.Context, sectors []api.SectorState, maddr address.Address) (map[abi.SectorID]struct{}, error)

	CheckAfter(maddr address.Address) *time.Timer
	Threshold(maddr address.Address) int
	EnableBatch(maddr address.Address) bool
}
