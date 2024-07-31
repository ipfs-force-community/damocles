package commitmgr

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

type Processor interface {
	Process(ctx context.Context, sectors []core.SectorState, mid abi.ActorID, ctrlAddr address.Address) error

	Expire(ctx context.Context, sectors []core.SectorState, mid abi.ActorID) (map[abi.SectorID]struct{}, error)

	CheckAfter(mid abi.ActorID) *time.Timer
	Threshold(mid abi.ActorID) int
	EnableBatch(mid abi.ActorID) bool
	ShouldBatch(mid abi.ActorID) bool
}
