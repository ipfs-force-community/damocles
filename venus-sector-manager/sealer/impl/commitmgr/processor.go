package commitmgr

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/sealer/api"
)

type Processor interface {
	Process(ctx context.Context, sectors []api.SectorState, mid abi.ActorID, ctrlAddr address.Address) error

	Expire(ctx context.Context, sectors []api.SectorState, mid abi.ActorID) (map[abi.SectorID]struct{}, error)

	CheckAfter(mid abi.ActorID) *time.Timer
	Threshold(mid abi.ActorID) int
	EnableBatch(mid abi.ActorID) bool
}
