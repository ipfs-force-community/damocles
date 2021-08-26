package poster

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/dtynn/venus-cluster/venus-sector-manager/api"
	"github.com/dtynn/venus-cluster/venus-sector-manager/modules"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("poster")

type PoSter struct {
	cfg      *modules.SafeConfig
	verifier api.Verifier
	prover   api.Prover
	indexer  api.SectorIndexer
	chain    chain.API
	rand     api.RandomnessAPI

	actors struct {
		sync.RWMutex
		handlers map[address.Address]*changeHandler
	}

	clock clock.Clock
}

func (p *PoSter) Run(ctx context.Context) {
	p.actors.RLock()
	for _, hdl := range p.actors.handlers {
		hdl.start()
	}
	p.actors.RUnlock()

	var notifs <-chan []*chain.HeadChange
	firstTime := true

	// not fine to panic after this point
	for {
		if notifs == nil {
			if !firstTime {
				select {
				case <-ctx.Done():
					return

				case <-time.After(10 * time.Second):

				}

			} else {
				firstTime = false
			}

			ch := p.chain.ChainNotify(ctx)
			if ch == nil {
				log.Error("get nil ChainNotify receiver")
				continue
			}

			notifs = ch
		}

		select {
		case <-ctx.Done():
			return

		case changes, ok := <-notifs:
			if !ok {
				log.Warn("window post scheduler notifs channel closed")
				notifs = nil
				continue
			}

			var lowest, highest *types.TipSet = nil, nil
			if len(changes) == 1 && changes[0].Type == chain.HCCurrent {
				highest = changes[0].Val
			} else {
				for _, change := range changes {
					if change.Val == nil {
						log.Errorf("change.Val was nil")
						continue
					}

					switch change.Type {
					case chain.HCRevert:
						lowest = change.Val
					case chain.HCApply:
						highest = change.Val
					}
				}
			}

			p.actors.RLock()
			for _, hdl := range p.actors.handlers {
				hdl.update(ctx, lowest, highest)
			}
			p.actors.RUnlock()
		}
	}
}
