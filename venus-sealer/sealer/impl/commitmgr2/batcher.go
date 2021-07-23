package commitmgr

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type Batcher struct {
	ctx   context.Context
	maddr address.Address

	pendingCh chan api.Sector

	processor Processor

	force, stop chan struct{}
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
		if len(pending) > 0 {
			mlog := log.With("miner", b.maddr)

			var processList []api.Sector
			if full || manual {
				processList = make([]api.Sector, len(pending))
				copy(processList, pending)

				pending = pending[:0]
			} else if tick {
				expired, err := b.processor.Expired(b.ctx, b.maddr, pending)
				if err != nil {
					mlog.Warnf("check expired sectors: %s", err)
				}

				if len(expired) > 0 {
					all := pending[:]
					remain := pending[:0]
					processList = make([]api.Sector, 0, len(all))
					for i, yes := range expired {
						if yes {
							processList = append(processList, all[i])
						} else {
							remain = append(remain, all[i])
						}
					}

					pending = remain
				}
			}

			if len(processList) > 0 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := b.processor.Process(b.ctx, wg, b.maddr, processList); err != nil {
						mlog.Errorf("process failed: %s", err)
					}
				}()
			}
		}

		if tick || full {
			timer = b.processor.CheckAfter(b.maddr)
		}
	}
}
