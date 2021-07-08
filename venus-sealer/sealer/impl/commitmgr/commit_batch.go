package commitmgr

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	"github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	"github.com/filecoin-project/venus/fixtures/networks"
	"github.com/filecoin-project/venus/pkg/specactors"
	"github.com/filecoin-project/venus/pkg/specactors/policy"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

type CommitBatcher struct {
	mctx  context.Context
	maddr address.Address
	api   SealingAPI

	msgClient venusMessager.IMessager
	prover    Prover
	verfi     Verifier

	deadlines map[abi.SectorNumber]time.Time
	todo      map[abi.SectorNumber]api.AggregateInput
	lk        sync.Mutex

	cfg struct {
		*sealer.Config
		confmgr.RLocker
	}

	ds              api.SectorsDatastore
	notify, stopped chan struct{}
	force           chan struct{}
}

type CommitBatcherCtor func(mctx context.Context, maddr address.Address, sealing SealingAPI,
	msgClient venusMessager.IMessager, prover Prover, verfi Verifier,
	cfg *sealer.Config, locker confmgr.RLocker, ds api.SectorsDatastore,
) *CommitBatcher

func (b *CommitBatcher) waitStop() {
	for range b.stopped {
	}
}

func (b *CommitBatcher) Add(ctx context.Context, s abi.SectorID) error {
	sector, err := b.ds.GetSector(ctx, s)
	if err != nil {
		return xerrors.Errorf("load sector from ds failed %w", err)
	}
	input := api.AggregateInput{
		Spt: sector.SectorType,
		Info: proof5.AggregateSealVerifyInfo{
			Number:                s.Number,
			Randomness:            sector.TicketValue,
			InteractiveRandomness: sector.SeedValue,
			SealedCID:             *sector.CommR,
			UnsealedCID:           *sector.CommD,
		},
		Proof: sector.Proof,
	}
	sn := s.Number
	cu, err := b.getCommitCutoff(sector)
	if err != nil {
		return xerrors.Errorf("temp error: get sector expire time failed")
	}
	b.lk.Lock()
	b.deadlines[sn] = cu
	b.todo[sn] = input

	select {
	case b.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	log.Infof("there are %d sectors in provecommit pool", len(b.todo))
	b.lk.Unlock()

	return nil
}

func (b *CommitBatcher) getCommitCutoff(si api.Sector) (time.Time, error) {
	tok, curEpoch, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return time.Now(), xerrors.Errorf("getting chain head: %s", err)
	}

	nv, err := b.api.StateNetworkVersion(b.mctx, tok)
	if err != nil {
		log.Errorf("getting network version: %s", err)
		return time.Now(), xerrors.Errorf("getting network version: %s", err)
	}

	pci, err := b.api.StateSectorPreCommitInfo(b.mctx, b.maddr, si.SectorID.Number, tok)
	if err != nil {
		log.Errorf("getting precommit info: %s", err)
		return time.Now(), err
	}

	cutoffEpoch := pci.PreCommitEpoch + policy.GetMaxProveCommitDuration(specactors.Version(nv), si.SectorType)

	for _, p := range si.Deals {
		proposal, err := b.api.StateMarketStorageDealProposal(b.mctx, p.ID, tok)
		if err != nil {
			return time.Now(), err
		}
		startEpoch := proposal.StartEpoch
		if startEpoch < cutoffEpoch {
			cutoffEpoch = startEpoch
		}
	}

	if cutoffEpoch <= curEpoch {
		return time.Now(), nil
	}

	return time.Now().Add(time.Duration(cutoffEpoch-curEpoch) * time.Duration(networks.Mainnet().Network.BlockDelay) * time.Second), nil
}

func NewCommitBatcher(mctx context.Context, maddr address.Address, sealing SealingAPI,
	msgClient venusMessager.IMessager, prover Prover, verfi Verifier,
	cfg *sealer.Config, locker confmgr.RLocker, ds api.SectorsDatastore,
) *CommitBatcher {
	return &CommitBatcher{
		mctx:      mctx,
		maddr:     maddr,
		api:       sealing,
		msgClient: msgClient,
		prover:    prover,
		verfi:     verfi,
		deadlines: map[abi.SectorNumber]time.Time{},
		todo:      map[abi.SectorNumber]api.AggregateInput{},
		lk:        sync.Mutex{},
		cfg: struct {
			*sealer.Config
			confmgr.RLocker
		}{cfg, locker},
		ds:      ds,
		notify:  make(chan struct{}, 1),
		force:   make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (b *CommitBatcher) run() {
	b.cfg.Lock()
	d := b.cfg.CommitmentManager.CommitBatchWait
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
			log.Errorf("commit batch process meet error")
		}

		if timer.Stop() {
			b.cfg.Lock()
			d = b.cfg.CommitmentManager.CommitBatchWait
			b.cfg.Unlock()
			timer.Reset(d)
		}
	}
}

// this should call under a lock outside
func (b *CommitBatcher) failSector(s abi.SectorID) error {
	sector, errGetSector := b.ds.GetSector(b.mctx, s)
	if errGetSector != nil {
		return errGetSector
	}
	sector.NeedSend = false
	b.cleanSector(s)
	return b.ds.PutSector(b.mctx, sector)
}

func (b *CommitBatcher) cleanSector(s abi.SectorID) {
	delete(b.todo, s.Number)
	delete(b.deadlines, s.Number)
}

func (b *CommitBatcher) process(forcePush, sendAboveMin, sendAboveMax bool, wg *sync.WaitGroup) error {
	tok, _, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return err
	}

	b.lk.Lock()
	defer b.lk.Unlock()
	b.cfg.Lock()
	maxBatch := b.cfg.CommitmentManager.MaxCommitBatch
	minBatch := b.cfg.CommitmentManager.MinCommitBatch
	cheaper := b.cfg.CommitmentManager.CheaperBaseFee
	slack := b.cfg.CommitmentManager.CommitBatchSlack
	b.cfg.Unlock()

	actorID, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return err
	}
	infos := []proof5.AggregateSealVerifyInfo{}
	for id, p := range b.todo {
		if len(infos) >= maxBatch {
			log.Infow("commit batch full")
			break
		}
		_, err := getSectorCollateral(b.mctx, b.api, b.maddr, id, tok)
		if err != nil {
			errClean := b.failSector(abi.SectorID{Miner: abi.ActorID(actorID), Number: id})
			if errClean != nil {
				return errClean
			}
			continue
		}
		infos = append(infos, p.Info)
	}

	if sendAboveMax && len(infos) < maxBatch {
		log.Info("pro-committer receive send signal but not reach max limit, will do nothing")
		return nil
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Number < infos[j].Number
	})

	baseFee, err := b.api.ChainBaseFee(b.mctx, tok)
	if err != nil {
		return xerrors.Errorf("get chain base fee failed: %w", err)
	}
	individually := baseFee.LessThanEqual(cheaper)

	// only push these close to failed sector
	if sendAboveMin && len(infos) < minBatch {
		infosTmp := []proof5.AggregateSealVerifyInfo{}
		for i := range infos {
			if b.deadlines[infos[i].Number].After(time.Now().Add(slack)) {
				infosTmp = append(infosTmp, infos[i])
			}
		}
		infos = infosTmp
	}

	// here all sector in infos shall be send whether aggregate or individually
	// so we clean sector info here
	wg.Add(1)
	for i := range infos {
		b.cleanSector(abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Number})
	}
	go func() {
		defer wg.Done()
		from, err := getProCommitControlAddress(b.maddr, b.cfg)
		if err != nil {
			log.Errorf("get procommit control address failed: %s", err.Error())
			return
		}

		if individually || len(infos) < miner.MinAggregatedSectors {
			for i := range infos {
				err := pushCommitSingle(b.mctx, b.ds, b.msgClient, from,
					abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Number},
					getMsgMeta(b.cfg), b.api)
				if err != nil {
					log.Errorf("send sector prove failed: %s", err.Error())
				}
			}
			return
		}
		ccid := cid.Undef
		defer func() {
			for i := range infos {
				sector, err := b.ds.GetSector(b.mctx, abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Number})
				if err != nil {
					log.Errorf("load sector %d %s from db failed: %s", infos[i].Number, b.maddr, err.Error())
					continue
				}
				if ccid != cid.Undef {
					sector.CommitCid = &ccid
				}
				sector.NeedSend = false
				err = b.ds.PutSector(b.mctx, sector)
				if err != nil {
					log.Errorf("put sector from db failed: %s", err.Error())
				}
			}
		}()

		params := &miner.ProveCommitAggregateParams{
			SectorNumbers: bitfield.New(),
		}
		proofs := make([][]byte, 0)
		collateral := big.Zero()
		for i := range infos {
			sc, err := getSectorCollateral(b.mctx, b.api, b.maddr, infos[i].Number, tok)
			if err != nil {
				// just log and keep the train on
				log.Errorf("get collateral failed: %s", err.Error())
			}
			collateral = big.Add(collateral, sc)
			params.SectorNumbers.Set(uint64(infos[i].Number))
			sector, err := b.ds.GetSector(b.mctx, abi.SectorID{Miner: abi.ActorID(actorID), Number: infos[i].Number})
			if err != nil {
				log.Errorf("load sector from db failed: %s", err.Error())
				return
			}
			proofs = append(proofs, sector.Proof)
		}

		params.AggregateProof, err = b.prover.AggregateSealProofs(proof5.AggregateSealVerifyProofAndInfos{
			Miner:          abi.ActorID(actorID),
			SealProof:      b.todo[infos[0].Number].Spt,
			AggregateProof: abi.RegisteredAggregationProof_SnarkPackV1,
			Infos:          infos,
		}, proofs)
		if err != nil {
			log.Errorf("aggregate sector failed: %s", err.Error())
			return
		}

		enc := new(bytes.Buffer)
		if err := params.MarshalCBOR(enc); err != nil {
			log.Errorf("couldn't serialize ProveCommitAggregateParams: %s", err.Error())
			return
		}

		ccid, err = pushMessage(b.mctx, from, b.maddr, collateral, builtin5.MethodsMiner.ProveCommitAggregate,
			b.msgClient, getMsgMeta(b.cfg), enc.Bytes())
		if err != nil {
			log.Errorf("push aggregate prove message failed: %s", err.Error())
		}
	}()
	return nil
}
