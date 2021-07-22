package commitmgr

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type Cfg struct {
	*sealer.Config
	confmgr.RLocker
}

type Processer interface {
	Process(ctx context.Context, msgClient venusMessager.IMessager, sectors []api.Sector, enableBatch bool, api SealingAPI,
		wg *sync.WaitGroup, maddr address.Address, prover *api.Prover, ds api.SectorsDatastore, config Cfg)

	PickTimeOutSector(ctx context.Context, sectors *[]api.Sector, api SealingAPI, config Cfg) ([]api.Sector, error)

	NewTimer(config Cfg) *time.Timer
	Threshold(config Cfg) int
	EnableBatch(config Cfg) bool
}

type Batcher struct {
	ctx   context.Context
	api   SealingAPI
	maddr address.Address

	ds        api.SectorsDatastore
	msgClient venusMessager.IMessager
	prover    *api.Prover

	pendingCh chan api.Sector

	cfg Cfg

	force, stop chan struct{}

	Processer
}

func (b *Batcher) waitStop() {
	<-b.stop
}

func (b *Batcher) Add(sector api.Sector) {
	b.pendingCh <- sector
}

func (b *Batcher) run() {
	timer := b.NewTimer(b.cfg)
	wg := &sync.WaitGroup{}

	defer func() {
		wg.Wait()
		close(b.stop)
	}()

	pending := []api.Sector{}

	for {
		checkInterval, manual := false, false
		processList := []api.Sector{}
		select {
		case <-b.ctx.Done():
			return
		case <-b.force:
			manual = true
		case <-timer.C:
			checkInterval = true
		case s := <-b.pendingCh:
			pending = append(pending, s)
		}

		if !b.EnableBatch(b.cfg) {
			processList = pending
			pending = []api.Sector{}
			wg.Add(1)
			go b.Process(b.ctx, b.msgClient, processList, false, b.api, wg, b.maddr, b.prover, b.ds, b.cfg)
			timer = b.NewTimer(b.cfg)
			continue
		}

		if manual || len(pending) >= b.Threshold(b.cfg) {
			processList = pending
			pending = []api.Sector{}
		} else if checkInterval && len(pending) < b.Threshold(b.cfg) {
			var err error
			processList, err = b.PickTimeOutSector(b.ctx, &pending, b.api, b.cfg)
			if err != nil {
				log.Errorf("pick time out sector failed: %s", err.Error())
				continue
			}
		}

		if len(processList) != 0 {
			wg.Add(1)
			go b.Process(b.ctx, b.msgClient, processList, true, b.api, wg, b.maddr, b.prover, b.ds, b.cfg)
			timer = b.NewTimer(b.cfg)
		}
	}
}

func NewBatcher(ctx context.Context, maddr address.Address, sealing SealingAPI,
	msgClient venusMessager.IMessager, prover *api.Prover, cfg *sealer.Config, locker confmgr.RLocker, ds api.SectorsDatastore, processer Processer) *Batcher {
	b := &Batcher{
		ctx:       ctx,
		api:       sealing,
		maddr:     maddr,
		ds:        ds,
		msgClient: msgClient,
		prover:    prover,
		pendingCh: make(chan api.Sector),
		cfg: Cfg{
			Config:  cfg,
			RLocker: locker,
		},
		force:     make(chan struct{}),
		stop:      make(chan struct{}),
		Processer: processer,
	}
	go b.run()
	return b
}
