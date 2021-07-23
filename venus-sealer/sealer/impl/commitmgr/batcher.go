package commitmgr

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type Cfg struct {
	*sealer.Config
	confmgr.RLocker
}

type Batcher struct {
	ctx   context.Context
	maddr address.Address

	pendingCh chan api.Sector

	force, stop chan struct{}

	processor Processor
}

func (b *Batcher) waitStop() {
	<-b.stop
}

func (b *Batcher) Add(sector api.Sector) {
	b.pendingCh <- sector
}

func (b *Batcher) run() {
	timer := b.processor.CheckAfter(b.maddr)
	wg := &sync.WaitGroup{}

	defer func() {
		wg.Wait()
		close(b.stop)
	}()

	pendingCap := b.processor.Threshold(b.maddr)
	if pendingCap > 128 {
		pendingCap /= 4
	}

	pending := make([]api.Sector, 0, pendingCap)

	for {
		tick, manual := false, false

		select {
		case <-b.ctx.Done():
			return
		case <-b.force:
			manual = true
		case <-timer.C:
			tick = true
		case s := <-b.pendingCh:
			pending = append(pending, s)
		}

		full := len(pending) >= b.processor.Threshold(b.maddr)
		cleanAll := false
		if len(pending) > 0 {
			mlog := log.With("miner", b.maddr)

			var processList []api.Sector
			if full || manual || !b.processor.EnableBatch(b.maddr) {
				processList = make([]api.Sector, len(pending))
				copy(processList, pending)

				pending = pending[:0]

				cleanAll = true
			} else if tick {
				expired, err := b.processor.Expire(b.ctx, pending, b.maddr)
				if err != nil {
					mlog.Warnf("check expired sectors: %s", err)
				}

				if len(expired) > 0 {
					remain := pending[:0]
					processList = make([]api.Sector, 0, len(pending))
					for i := range pending {
						if _, ok := expired[pending[i].SectorID]; ok {
							processList = append(processList, pending[i])
						} else {
							remain = append(remain, pending[i])
						}
					}

					pending = remain
				}
			}

			if len(processList) > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := b.processor.Process(b.ctx, processList, b.maddr); err != nil {
						mlog.Errorf("process failed: %s", err)
					}
				}()
			}
		}

		if tick || cleanAll {
			timer = b.processor.CheckAfter(b.maddr)
		}
	}
}

func NewBatcher(ctx context.Context, maddr address.Address, processer Processor) *Batcher {
	b := &Batcher{
		ctx:       ctx,
		maddr:     maddr,
		pendingCh: make(chan api.Sector),
		force:     make(chan struct{}),
		stop:      make(chan struct{}),
		processor: processer,
	}
	go b.run()
	return b
}
