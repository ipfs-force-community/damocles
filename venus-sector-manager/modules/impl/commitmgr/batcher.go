package commitmgr

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

type Batcher struct {
	ctx      context.Context
	mid      abi.ActorID
	ctrlAddr address.Address

	pendingCh chan api.SectorState

	force, stop chan struct{}

	processor Processor

	log *logging.ZapLogger
}

func (b *Batcher) waitStop() {
	<-b.stop
}

func (b *Batcher) Add(sector api.SectorState) {
	b.pendingCh <- sector
}

func (b *Batcher) run() {
	timer := b.processor.CheckAfter(b.mid)
	wg := &sync.WaitGroup{}

	defer func() {
		wg.Wait()
		close(b.stop)
	}()

	pendingCap := b.processor.Threshold(b.mid)
	if pendingCap > 128 {
		pendingCap /= 4
	}

	pending := make([]api.SectorState, 0, pendingCap)

	for {
		tick, manual := false, false

		select {
		case <-b.ctx.Done():
			return
		case <-b.force:
			manual = true
			b.log.Info("receive manual sig, checking processlist")
		case <-timer.C:
			tick = true
			b.log.Info("time run out, checking processlist")
		case s := <-b.pendingCh:
			pending = append(pending, s)
			b.log.Info("new sector reaches, checking processlist")
		}

		full := len(pending) >= b.processor.Threshold(b.mid)
		cleanAll := false
		if len(pending) > 0 {
			var processList []api.SectorState
			if full || manual || !b.processor.EnableBatch(b.mid) {
				processList = make([]api.SectorState, len(pending))
				copy(processList, pending)

				pending = pending[:0]

				cleanAll = true
			} else if tick {
				expired, err := b.processor.Expire(b.ctx, pending, b.mid)
				if err != nil {
					b.log.Warnf("check expired sectors: %s", err)
				}

				if len(expired) > 0 {
					remain := pending[:0]
					processList = make([]api.SectorState, 0, len(pending))
					for i := range pending {
						if _, ok := expired[pending[i].ID]; ok {
							processList = append(processList, pending[i])
						} else {
							remain = append(remain, pending[i])
						}
					}

					pending = remain
				}
			}

			if len(processList) > 0 {
				b.log.Debugw("will process sectors", "len", len(processList), "full", full, "manual", manual, "all", cleanAll, "tick", tick)
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := b.processor.Process(b.ctx, processList, b.mid, b.ctrlAddr); err != nil {
						b.log.Errorf("process failed: %s", err)
					}
				}()
			}
		}

		if tick || cleanAll {
			timer.Stop()
			timer = b.processor.CheckAfter(b.mid)
		}
	}
}

func NewBatcher(ctx context.Context, mid abi.ActorID, ctrlAddr address.Address, processer Processor, l *logging.ZapLogger) *Batcher {
	b := &Batcher{
		ctx:       ctx,
		mid:       mid,
		ctrlAddr:  ctrlAddr,
		pendingCh: make(chan api.SectorState),
		force:     make(chan struct{}),
		stop:      make(chan struct{}),
		processor: processer,
		log:       log.With("miner", mid),
	}
	go b.run()

	b.log.Debug("batcher init")
	return b
}
