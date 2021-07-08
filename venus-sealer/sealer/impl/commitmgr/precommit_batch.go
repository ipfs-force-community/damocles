package commitmgr

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	"github.com/filecoin-project/venus/fixtures/networks"
	"github.com/filecoin-project/venus/pkg/specactors/policy"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type PreCommitBatcher struct {
	mctx  context.Context
	maddr address.Address
	api   SealingAPI

	msgClient venusMessager.IMessager

	deadlines map[abi.SectorNumber]time.Time
	todo      map[abi.SectorNumber]api.PreCommitEntry
	lk        sync.Mutex

	cfg struct {
		*sealer.Config
		confmgr.RLocker
	}

	ds              api.SectorsDatastore
	notify, stopped chan struct{}
	force           chan chan *cid.Cid
}

type PreCommitBatcherCtor func(mctx context.Context, maddr address.Address, sealing SealingAPI,
	msgClient venusMessager.IMessager, cfg *sealer.Config, locker confmgr.RLocker, ds api.SectorsDatastore) *PreCommitBatcher

func (pcb *PreCommitBatcher) getSectorDeadline(curEpoch abi.ChainEpoch, si api.Sector) (time.Time, error) {
	deadlineEpoch := si.TicketEpoch + policy.MaxPreCommitRandomnessLookback
	tok, curEpoch, err := pcb.api.ChainHead(pcb.mctx)
	if err != nil {
		return time.Now(), err
	}
	for _, p := range si.Deals {
		proposal, err := pcb.api.StateMarketStorageDealProposal(pcb.mctx, p.ID, tok)
		if err != nil {
			return time.Now(), err
		}
		startEpoch := proposal.StartEpoch
		if startEpoch < deadlineEpoch {
			deadlineEpoch = startEpoch
		}
	}

	if deadlineEpoch <= curEpoch {
		return time.Now(), nil
	}

	return time.Now().Add(time.Duration(deadlineEpoch-curEpoch) * time.Duration(networks.Mainnet().Network.BlockDelay) * time.Second), nil
}

func (pcb *PreCommitBatcher) waitStop() {
	for range pcb.stopped {

	}
}

func (pcb *PreCommitBatcher) Add(ctx context.Context, s abi.SectorID) error {
	sn := s.Number
	_, curEpoch, err := pcb.api.ChainHead(pcb.mctx)
	if err != nil {
		log.Errorf("getting chain head: %s", err)
		return xerrors.Errorf("api error: get chain head failed :%w", err)
	}
	sector, err := pcb.ds.GetSector(ctx, s)
	if err != nil {
		return err
	}
	params, deposit, _, err := preCommitParams(ctx, pcb.api, sector)
	if err != nil {
		return err
	}
	pcb.lk.Lock()
	pcb.deadlines[sn], err = pcb.getSectorDeadline(curEpoch, sector)
	pcb.todo[sn] = api.PreCommitEntry{
		Deposit: deposit,
		Pci:     params,
	}

	select {
	case pcb.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	log.Infof("there are %d sectors in precommit pool", len(pcb.todo))
	pcb.lk.Unlock()
	return nil
}

func NewPreCommitBatcher(mctx context.Context, maddr address.Address, sealing SealingAPI,
	msgClient venusMessager.IMessager, cfg *sealer.Config, locker confmgr.RLocker, ds api.SectorsDatastore) *PreCommitBatcher {
	return &PreCommitBatcher{
		mctx:      mctx,
		maddr:     maddr,
		api:       sealing,
		msgClient: msgClient,
		deadlines: map[abi.SectorNumber]time.Time{},
		todo:      map[abi.SectorNumber]api.PreCommitEntry{},
		lk:        sync.Mutex{},
		cfg: struct {
			*sealer.Config
			confmgr.RLocker
		}{cfg, locker},
		ds:      ds,
		notify:  make(chan struct{}, 1),
		force:   make(chan chan *cid.Cid),
		stopped: make(chan struct{}),
	}
}

// this should call under a lock outside
func (b *PreCommitBatcher) failSector(s abi.SectorID) error {
	sector, errGetSector := b.ds.GetSector(b.mctx, s)
	if errGetSector != nil {
		return errGetSector
	}
	sector.NeedSend = false
	b.cleanSector(s)
	return b.ds.PutSector(b.mctx, sector)
}

func (b *PreCommitBatcher) cleanSector(s abi.SectorID) {
	delete(b.todo, s.Number)
	delete(b.deadlines, s.Number)
}
func (b *PreCommitBatcher) run() {
	b.cfg.Lock()
	d := b.cfg.CommitmentManager.PreCommitBatchWait
	b.cfg.Unlock()
	timer := time.NewTimer(d)

	wg := sync.WaitGroup{}

	defer func() {
		wg.Done()
		close(b.stopped)
	}()

	for {
		var forcePush, sendAboveMin, sendAboveMax bool
		select {
		case <-timer.C:
			sendAboveMin = true
		case <-b.mctx.Done():
			return
		case <-b.notify:
			sendAboveMax = true
		case <-b.force:
			forcePush = true
		}

		err := b.process(forcePush, sendAboveMin, sendAboveMax, &wg)
		if err != nil {
			log.Errorf("precomit batch process meet error")
		}

		if timer.Stop() {
			b.cfg.Lock()
			d = b.cfg.CommitmentManager.PreCommitBatchWait
			b.cfg.Unlock()
			timer.Reset(d)
		}
	}
}

func (b *PreCommitBatcher) process(forcePush, sendAboveMin, sendAboveMax bool, wg *sync.WaitGroup) error {
	b.lk.Lock()
	defer b.lk.Unlock()
	b.cfg.Lock()
	maxBatch := b.cfg.CommitmentManager.MaxPreCommitBatch
	minBatch := b.cfg.CommitmentManager.MinCommitBatch
	slack := b.cfg.CommitmentManager.PreCommitBatchSlack
	b.cfg.Unlock()

	actorID, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return err
	}
	infos := []api.PreCommitEntry{}
	for _, p := range b.todo {
		if len(infos) >= maxBatch {
			log.Infow("pre commit batch full")
			break
		}
		infos = append(infos, p)
	}

	if sendAboveMax && len(infos) < maxBatch {
		log.Info("pre-committer receive send signal but not reach max limit, will do nothing")
		return nil
	}

	// only push these close to failed sector
	if sendAboveMin && len(infos) < minBatch {
		infosTmp := []api.PreCommitEntry{}
		for i := range infos {
			if b.deadlines[infos[i].Pci.SectorNumber].After(time.Now().Add(slack)) {
				infosTmp = append(infosTmp, infos[i])
			}
		}
		infos = infosTmp
	}

	// here all sector in infos shall be send whether aggregate or individually
	// so we clean sector info here
	wg.Add(1)
	for i := range infos {
		b.cleanSector(abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Pci.SectorNumber})
	}
	go func() {
		defer wg.Done()
		from, err := getPreCommitControlAddress(b.maddr, b.cfg)
		if err != nil {
			log.Errorf("get procommit control address failed: %s", err.Error())
			return
		}

		// precommit always good that batch send
		ccid := cid.Undef
		defer func() {
			for i := range infos {
				sector, err := b.ds.GetSector(b.mctx, abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Pci.SectorNumber})
				if err != nil {
					log.Errorf("load sector %d %s from db failed: %s", infos[i].Pci.SectorNumber, b.maddr, err.Error())
					continue
				}
				if ccid != cid.Undef {
					sector.PreCommitCid = &ccid
				}
				sector.NeedSend = false
				err = b.ds.PutSector(b.mctx, sector)
				if err != nil {
					log.Errorf("put sector from db failed: %s", err.Error())
				}
			}
		}()

		params := miner.PreCommitSectorBatchParams{}

		deposit := big.Zero()

		for i := range infos {
			params.Sectors = append(params.Sectors, *infos[i].Pci)
			deposit = big.Add(deposit, infos[i].Deposit)
		}

		enc := new(bytes.Buffer)
		if err := params.MarshalCBOR(enc); err != nil {
			log.Errorf("couldn't serialize PreCommitSectorBatchParams: %s", err.Error())
			return
		}

		ccid, err = pushMessage(b.mctx, from, b.maddr, deposit, builtin5.MethodsMiner.PreCommitSectorBatch,
			b.msgClient, getMsgMeta(b.cfg), enc.Bytes())
		if err != nil {
			log.Errorf("push batch precommit message failed: %s", err.Error())
		}
	}()
	return nil
}
