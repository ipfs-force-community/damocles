package chain

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var eventLog = logging.New("chain-eventbus")

type eventBusCallback struct {
	h abi.ChainEpoch
	f func(ts *types.TipSet)
}

func NewEventBus(ctx context.Context, capi API, interval time.Duration) (*EventBus, error) {
	ctx, cancel := context.WithCancel(ctx)
	eb := &EventBus{
		ctx:    ctx,
		cancel: cancel,

		interval: interval,
		capi:     capi,
	}

	return eb, nil
}

type EventBus struct {
	ctx    context.Context
	cancel context.CancelFunc

	interval time.Duration
	capi     API

	cbsMu sync.Mutex
	cbs   []*eventBusCallback
}

func (e *EventBus) Run() {
	timer := time.NewTimer(e.interval)
	defer timer.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return

		case <-timer.C:
			ts, err := e.capi.ChainHead(e.ctx)
			if err != nil {
				eventLog.Warnf("get chain head: %s", err)
			} else if ts != nil {
				e.trigger(ts)
			}

			timer.Reset(e.interval)
		}
	}
}

func (e *EventBus) Stop() {
	e.cancel()
}

// exec callback func when receive tipset with epoch >= height + confidential
func (e *EventBus) At(ctx context.Context, height abi.ChainEpoch, confidential abi.ChainEpoch, callback func(ts *types.TipSet)) {
	if callback == nil {
		return
	}

	cb := &eventBusCallback{
		h: height + confidential,
		f: callback,
	}

	e.cbsMu.Lock()
	e.cbs = append(e.cbs, cb)
	// desc
	sort.Slice(e.cbs, func(i, j int) bool {
		return e.cbs[i].h > e.cbs[j].h
	})
	e.cbsMu.Unlock()
	eventLog.Debugw("event registered", "h", height, "c", confidential)
}

func (e *EventBus) trigger(ts *types.TipSet) {
	e.cbsMu.Lock()
	defer e.cbsMu.Unlock()

	count := len(e.cbs)
	if count == 0 {
		return
	}

	tsh := ts.Height()

	start := -1
	for i := 0; i < count; i++ {
		if e.cbs[i].h <= tsh {
			start = i
			break
		}
	}

	if start < 0 {
		return
	}

	now := time.Now()
	for i := start; i < count; i++ {
		cb := e.cbs[i].f
		go cb(ts)
		e.cbs[i] = nil
	}

	e.cbs = e.cbs[:start]
	eventLog.Debugw("events triggered", "count", count-start, "elapsed", time.Since(now).String())
}
