package commitmgr

import (
	"context"
	"sync"
	"time"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/filecoin-project/go-address"
)

type Processor interface {
	Process(ctx context.Context, wg *sync.WaitGroup, maddr address.Address, sectors []api.Sector) error
	Expired(ctx context.Context, maddr address.Address, sectors []api.Sector) ([]bool, error)
	CheckAfter(maddr address.Address) *time.Timer
	Threshold(maddr address.Address) int
}
