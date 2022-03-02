package poster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

var log = logging.New("poster")

func NewPoSter(
	ctx context.Context,
	cfg *modules.SafeConfig,
	verifier api.Verifier,
	prover api.Prover,
	indexer api.SectorIndexer,
	sectorTracker api.SectorTracker,
	capi chain.API,
	rand api.RandomnessAPI,
	mapi messager.API,
) (*PoSter, error) {
	p := &PoSter{
		cfg:           cfg,
		verifier:      verifier,
		prover:        prover,
		indexer:       indexer,
		sectorTracker: sectorTracker,
		chain:         capi,
		rand:          rand,
		msg:           mapi,
	}

	p.actors.handlers = map[address.Address]*changeHandler{}

	cfg.Lock()
	actors := cfg.PoSt.Actors
	cfg.Unlock()

	for key := range actors {
		mid, err := modules.ActorIDFromConfigKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse actor id from %s: %w", key, err)
		}

		sched, err := newScheduler(ctx, mid, p.cfg, p.verifier, p.prover, p.indexer, p.sectorTracker, p.chain, p.rand, p.msg)
		if err != nil {
			return nil, fmt.Errorf("construct scheduler for actor %d: %w", mid, err)
		}

		p.actors.handlers[sched.actor.Addr] = newChangeHandler(sched, sched.actor.Addr)
	}

	return p, nil
}

type PoSter struct {
	cfg           *modules.SafeConfig
	verifier      api.Verifier
	prover        api.Prover
	indexer       api.SectorIndexer
	sectorTracker api.SectorTracker
	chain         chain.API
	rand          api.RandomnessAPI
	msg           messager.API

	actors struct {
		sync.RWMutex
		handlers map[address.Address]*changeHandler
	}
}

func (p *PoSter) Run(ctx context.Context) {
	log.Info("poster loop start")
	defer log.Info("poster loop stop")

	p.actors.RLock()
	handlers := p.actors.handlers
	p.actors.RUnlock()

	if len(handlers) == 0 {
		log.Warn("no actor setup")
		return
	}

	for _, hdl := range handlers {
		hdl.start()
	}

	var notifs <-chan []*chain.HeadChange
	firstTime := true

	reconnectWait := 10 * time.Second

	// not fine to panic after this point
	for {
		if notifs == nil {
			if !firstTime {
				log.Warnf("try to reconnect after %s", reconnectWait)
				select {
				case <-ctx.Done():
					return

				case <-time.After(reconnectWait):

				}

			} else {
				firstTime = false
			}

			ch, err := p.chain.ChainNotify(ctx)
			if err != nil {
				log.Errorf("get ChainNotify error: %w", err)
				continue
			}
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
