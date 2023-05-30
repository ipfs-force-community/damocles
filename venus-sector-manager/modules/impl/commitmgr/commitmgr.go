package commitmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

var (
	errMsgPublishAttemptFailed  = "attempt to publish message but failed"
	errMsgReceiptNotFound       = "receipt not found for on-chain message"
	errMsgSectorAllocated       = "sector already allocated"
	errMsgPreCommitInfoNotFound = "pre-commit info not found on chain"
	errMsgSectorInfoNotFound    = "sector info not found on chain"
)

var log = logging.New("commitmgr")

type CommitmentMgrImpl struct {
	ctx context.Context

	msgClient messager.API

	stateMgr SealingAPI
	minerAPI core.MinerAPI

	smgr core.SectorStateManager

	cfg *modules.SafeConfig

	commitBatcher    map[abi.ActorID]*Batcher
	preCommitBatcher map[abi.ActorID]*Batcher
	terminateBatcher map[abi.ActorID]*Batcher

	prePendingChan       chan core.SectorState
	proPendingChan       chan core.SectorState
	terminatePendingChan chan core.SectorState

	verif  core.Verifier
	prover core.Prover

	stopOnce sync.Once
	stop     chan struct{}
}

func NewCommitmentMgr(ctx context.Context, commitAPI messager.API, stateMgr SealingAPI, minerAPI core.MinerAPI, smgr core.SectorStateManager,
	cfg *modules.SafeConfig, verif core.Verifier, prover core.Prover,
) (*CommitmentMgrImpl, error) {
	prePendingChan := make(chan core.SectorState, 1024)
	proPendingChan := make(chan core.SectorState, 1024)
	terminatePendingChan := make(chan core.SectorState, 1024)

	mgr := CommitmentMgrImpl{
		ctx:       ctx,
		msgClient: commitAPI,
		stateMgr:  stateMgr,
		minerAPI:  minerAPI,
		smgr:      smgr,
		cfg:       cfg,

		commitBatcher:    map[abi.ActorID]*Batcher{},
		preCommitBatcher: map[abi.ActorID]*Batcher{},
		terminateBatcher: map[abi.ActorID]*Batcher{},

		prePendingChan:       prePendingChan,
		proPendingChan:       proPendingChan,
		terminatePendingChan: terminatePendingChan,

		verif:  verif,
		prover: prover,
		stop:   make(chan struct{}),
	}

	return &mgr, nil
}

func updateSector(ctx context.Context, stmgr core.SectorStateManager, sector []core.SectorState, plog *logging.ZapLogger) {
	sectorID := make([]abi.SectorID, len(sector))
	for i := range sector {
		sectorID[i] = sector[i].ID
		sector[i].MessageInfo.NeedSend = false
		err := stmgr.Update(ctx, sector[i].ID, core.WorkerOnline, sector[i].MessageInfo)
		if err != nil {
			plog.With("sector", sector[i].ID.Number).Errorf("Update sector MessageInfo failed: %s", err)
		}
	}

	plog.Infof("Process sectors %v finished", sectorID)
}

func pushMessage(ctx context.Context, from address.Address, mid abi.ActorID, value abi.TokenAmount, method abi.MethodNum,
	msgClient messager.API, feeCfg *modules.FeeConfig, params []byte, mlog *logging.ZapLogger) (cid.Cid, error) {

	to, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return cid.Undef, err
	}

	spec := feeCfg.GetSendSpec()
	msg := types.Message{
		To:        to,
		From:      from,
		Value:     value,
		Method:    method,
		Params:    params,
		GasFeeCap: feeCfg.GetGasFeeCap().Std(),
	}

	bk, err := msg.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}
	mb := bk.RawData()

	mlog = mlog.With("from", from.String(), "to", to.String(), "method", method, "raw-mcid", bk.Cid())

	var mcid cid.Cid

	for i := 0; ; i++ {
		r := []byte{byte(i)}
		r = append(r, mb...)
		mid, err := NewMIdFromBytes(r)
		if err != nil {
			return cid.Undef, err
		}

		has, err := msgClient.HasMessageByUid(ctx, mid.String())
		if err != nil {
			return cid.Undef, err
		}

		mlog.Debugw("check if message exists", "tried", i, "has", has, "msgid", mid.String())
		if !has {
			mcid = mid
			break
		}
	}

	uid, err := msgClient.PushMessageWithId(ctx, mcid.String(), &msg, &spec)
	if err != nil {
		return cid.Undef, fmt.Errorf("push message with id failed: %w", err)
	}

	if uid != mcid.String() {
		return cid.Undef, errors.New("mcid not equal to uid, its out of control")
	}

	mlog.Infow("message sent", "mcid", uid)
	return mcid, nil
}

func NewMIdFromBytes(seed []byte) (cid.Cid, error) {
	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.BLAKE2B_MAX,
		MhLength: -1, // default length
	}

	// And then feed it some data
	c, err := pref.Sum(seed)
	if err != nil {
		return cid.Undef, err
	}
	return c, nil
}

func (c *CommitmentMgrImpl) Run(ctx context.Context) {
	go c.startPreLoop()
	go c.startProLoop()
	go c.startTerminateLoop(ctx)

	go c.restartSector(ctx)
}

func (c *CommitmentMgrImpl) Stop() {
	log.Info("stop commitment manager")
	c.stopOnce.Do(func() {
		close(c.prePendingChan)
		close(c.proPendingChan)
		close(c.terminatePendingChan)
		close(c.stop)

		for i := range c.commitBatcher {
			c.commitBatcher[i].waitStop()
		}
		for i := range c.preCommitBatcher {
			c.preCommitBatcher[i].waitStop()
		}
		for i := range c.terminateBatcher {
			c.terminateBatcher[i].waitStop()
		}
	})
}

func (c *CommitmentMgrImpl) preSender(mid abi.ActorID) (address.Address, error) {
	mcfg, err := c.cfg.MinerConfig(mid)
	if err != nil {
		return address.Undef, fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	if !mcfg.Commitment.Pre.Sender.Valid() {
		return address.Undef, fmt.Errorf("sender address not valid")
	}

	return mcfg.Commitment.Pre.Sender.Std(), nil
}

func (c *CommitmentMgrImpl) proveSender(mid abi.ActorID) (address.Address, error) {
	mcfg, err := c.cfg.MinerConfig(mid)
	if err != nil {
		return address.Undef, fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	if !mcfg.Commitment.Prove.Sender.Valid() {
		return address.Undef, fmt.Errorf("sender address not valid")
	}

	return mcfg.Commitment.Prove.Sender.Std(), nil
}

func (c *CommitmentMgrImpl) terminateSender(mid abi.ActorID) (address.Address, error) {
	mcfg, err := c.cfg.MinerConfig(mid)
	if err != nil {
		return address.Undef, fmt.Errorf("get miner config for %d: %w", mid, err)
	}

	if !mcfg.Commitment.Terminate.Sender.Valid() {
		return address.Undef, fmt.Errorf("sender address not valid")
	}

	return mcfg.Commitment.Terminate.Sender.Std(), nil
}

func (c *CommitmentMgrImpl) startPreLoop() {
	llog := log.With("loop", "pre")

	llog.Info("pending loop start")
	defer llog.Info("pending loop stop")

	for s := range c.prePendingChan {
		miner := s.ID.Miner
		if _, ok := c.preCommitBatcher[miner]; !ok {
			_, err := address.NewIDAddress(uint64(miner))
			if err != nil {
				llog.Errorf("trans miner from actor %d to address failed: %s", miner, err)
				continue
			}

			sender, err := c.preSender(miner)
			if err != nil {
				llog.Errorf("get sender address: %s", err)
				continue
			}

			c.preCommitBatcher[miner] = NewBatcher(c.ctx, miner, sender, PreCommitProcessor{
				api:       c.stateMgr,
				mapi:      c.minerAPI,
				msgClient: c.msgClient,
				smgr:      c.smgr,
				config:    c.cfg,
			}, llog)
		}

		c.preCommitBatcher[miner].Add(s)
	}
}

func (c *CommitmentMgrImpl) startProLoop() {
	llog := log.With("loop", "pro")

	llog.Info("pending loop start")
	defer llog.Info("pending loop stop")

	for s := range c.proPendingChan {
		miner := s.ID.Miner
		if _, ok := c.commitBatcher[miner]; !ok {
			_, err := address.NewIDAddress(uint64(miner))
			if err != nil {
				llog.Errorf("trans miner from actor %d to address failed: %s", miner, err)
				continue
			}

			sender, err := c.proveSender(miner)
			if err != nil {
				llog.Errorf("get sender address: %s", err)
				continue
			}

			c.commitBatcher[miner] = NewBatcher(c.ctx, miner, sender, CommitProcessor{
				api:       c.stateMgr,
				msgClient: c.msgClient,
				smgr:      c.smgr,
				config:    c.cfg,
				prover:    c.prover,
			}, llog)
		}

		c.commitBatcher[miner].Add(s)
	}
}

func (c *CommitmentMgrImpl) pollTerminateState(ctx context.Context, sector *core.SectorState) {
	mlog := log.With("sector-id", sector.ID, "stage", "terminate-commit")
	mlog.Debug("poll start")

	for {
		select {
		case <-time.After(time.Minute):
		case <-ctx.Done():
			return
		}

		sector, err := c.smgr.Load(ctx, sector.ID, core.WorkerOffline)
		if err != nil {
			mlog.Error(err.Error())
			break
		}

		if sector.TerminateInfo.TerminateCid == nil {
			continue
		}

		msg, err := c.msgClient.GetMessageByUid(ctx, sector.TerminateInfo.TerminateCid.String())
		if err != nil {
			mlog.Error(err.Error())
			break
		}

		state, _ := c.handleMessage(ctx, sector.ID.Miner, msg, mlog)
		if state == core.OnChainStateLanded {
			sector.TerminateInfo.TerminatedAt = abi.ChainEpoch(msg.Height)

			err := c.smgr.Update(ctx, sector.ID, core.WorkerOffline, sector.TerminateInfo)
			if err != nil {
				mlog.Errorf("Update sector TerminateInfo failed: %s", err)
			}

			mlog.Debug("poll finished")
			break
		} else if state == core.OnChainStateFailed {
			sector.TerminateInfo.AddedHeight = 0

			err := c.smgr.Update(ctx, sector.ID, core.WorkerOffline, sector.TerminateInfo)
			if err != nil {
				mlog.Errorf("Update sector TerminateInfo failed: %s", err)
			}

			mlog.Warn("terminate msg failed, check whether need re terminate")
			mlog.Debug("poll finished")
			break
		}
	}
}

func (c *CommitmentMgrImpl) startTerminateLoop(ctx context.Context) {
	llog := log.With("loop", "terminate")

	llog.Info("pending loop start")
	defer llog.Info("pending loop stop")

	for s := range c.terminatePendingChan {
		miner := s.ID.Miner
		if _, ok := c.terminateBatcher[miner]; !ok {
			sender, err := c.terminateSender(miner)
			if err != nil {
				llog.Errorf("get sender address: %s", err)
				continue
			}

			c.terminateBatcher[miner] = NewBatcher(c.ctx, miner, sender, TerminateProcessor{
				api:       c.stateMgr,
				msgClient: c.msgClient,
				smgr:      c.smgr,
				config:    c.cfg,
				prover:    c.prover,
			}, llog)
		}

		go c.pollTerminateState(ctx, &s)

		c.terminateBatcher[miner].Add(s)
	}
}

func (c *CommitmentMgrImpl) restartSector(ctx context.Context) {
	sectors, err := c.smgr.All(ctx, core.WorkerOnline, core.SectorWorkerJobSealing)
	if err != nil {
		log.Errorf("load all sector from db failed: %s", err)
		return
	}

	log.Debugw("previous sectors loaded", "count", len(sectors))

	for i := range sectors {
		if sectors[i].PendingForSealingCommitment() {
			if sectors[i].MessageInfo.PreCommitCid == nil {
				c.prePendingChan <- *sectors[i]
			} else {
				c.proPendingChan <- *sectors[i]
			}
		}
	}

	offlineSectors, err := c.smgr.All(ctx, core.WorkerOffline, core.SectorWorkerJobAll)
	if err != nil {
		log.Errorf("load all offline sector from db failed: %s", err)
		return
	}

	log.Debugw("previous offline sectors loaded", "count", len(offlineSectors))

	for i := range offlineSectors {
		if offlineSectors[i].PendingForTerminateCommitment() {
			if offlineSectors[i].TerminateInfo.TerminateCid != nil {
				go c.pollTerminateState(ctx, offlineSectors[i])
			} else {
				c.terminatePendingChan <- *offlineSectors[i]
			}
		}
	}
}

func (c *CommitmentMgrImpl) SubmitPreCommit(ctx context.Context, id abi.SectorID, info core.PreCommitInfo, hardReset bool) (core.SubmitPreCommitResp, error) {
	_, err := c.preSender(id.Miner)
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}

	sector, err := c.smgr.Load(ctx, id, core.WorkerOnline)
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		errMsg := err.Error()
		return core.SubmitPreCommitResp{Res: core.SubmitRejected, Desc: &errMsg}, nil
	}

	if sector.Pre != nil && !hardReset {
		preInfoChanged := (sector.Pre.CommD != info.CommD) ||
			(sector.Pre.CommR != info.CommR) ||
			(sector.Pre.Ticket.Epoch != info.Ticket.Epoch || !bytes.Equal(sector.Pre.Ticket.Ticket, info.Ticket.Ticket))

		if preInfoChanged {
			return core.SubmitPreCommitResp{Res: core.SubmitMismatchedSubmission}, nil
		}

		return core.SubmitPreCommitResp{Res: core.SubmitAccepted}, nil
	}

	sector.Pre = &info

	err = checkPrecommit(ctx, maddr, *sector, c.stateMgr)
	if err != nil {
		switch err.(type) {
		case *ErrAPI:
			return core.SubmitPreCommitResp{}, err

		case *ErrPrecommitOnChain:
			return core.SubmitPreCommitResp{Res: core.SubmitAccepted}, nil

		case *ErrBadCommD:
			// When encountering `ErrBadCommD` error, venus-worker should not release these deals,
			// as it is highly likely that the same error will occur when these deals are released and re claimed by venus-cluster.
			// The current approach is to return an error of type `SubmitMismatchedSubmission``, causing venus-worker to pause the sealing_thread
			// to waiting for manual processing
			errMsg := err.Error()
			return core.SubmitPreCommitResp{Res: core.SubmitMismatchedSubmission, Desc: &errMsg}, nil

		default:
			errMsg := err.Error()
			return core.SubmitPreCommitResp{Res: core.SubmitRejected, Desc: &errMsg}, nil
		}
	}

	sector.MessageInfo.NeedSend = true
	sector.MessageInfo.PreCommitCid = nil
	err = c.smgr.Update(ctx, sector.ID, core.WorkerOnline, sector.Pre, sector.MessageInfo)
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}

	go func() {
		c.prePendingChan <- *sector
	}()

	return core.SubmitPreCommitResp{
		Res: core.SubmitAccepted,
	}, nil
}

func (c *CommitmentMgrImpl) PreCommitState(ctx context.Context, id abi.SectorID) (core.PollPreCommitStateResp, error) {
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return core.PollPreCommitStateResp{}, err
	}

	sector, err := c.smgr.Load(ctx, id, core.WorkerOnline)
	if err != nil {
		return core.PollPreCommitStateResp{}, err
	}

	// pending
	if sector.MessageInfo.PreCommitCid == nil {
		if sector.MessageInfo.NeedSend {
			return core.PollPreCommitStateResp{State: core.OnChainStatePending}, nil
		}

		return core.PollPreCommitStateResp{State: core.OnChainStateFailed, Desc: &errMsgPublishAttemptFailed}, nil
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.PreCommitCid.String())
	if err != nil {
		return core.PollPreCommitStateResp{}, err
	}

	mlog := log.With("sector-id", id, "stage", "pre-commit")

	state, maybe := c.handleMessage(ctx, id.Miner, msg, mlog)
	if state == core.OnChainStateLanded {
		pci, err := c.stateMgr.StateSectorPreCommitInfo(ctx, maddr, id.Number, nil)
		if err == ErrSectorAllocated {
			return core.PollPreCommitStateResp{State: core.OnChainStateShouldAbort, Desc: &errMsgSectorAllocated}, nil
		}

		if err != nil {
			return core.PollPreCommitStateResp{}, err
		}

		if pci == nil {
			return core.PollPreCommitStateResp{State: core.OnChainStateShouldAbort, Desc: &errMsgPreCommitInfoNotFound}, nil
		}
	}

	return core.PollPreCommitStateResp{State: state, Desc: maybe}, nil
}

func (c *CommitmentMgrImpl) SubmitProof(ctx context.Context, id abi.SectorID, info core.ProofInfo, hardReset bool) (core.SubmitProofResp, error) {
	_, err := c.proveSender(id.Miner)
	if err != nil {
		return core.SubmitProofResp{}, err
	}

	sector, err := c.smgr.Load(ctx, id, core.WorkerOnline)
	if err != nil {
		return core.SubmitProofResp{}, err
	}

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		errMsg := err.Error()
		return core.SubmitProofResp{Res: core.SubmitRejected, Desc: &errMsg}, nil
	}

	if sector.Pre == nil {
		return core.SubmitProofResp{Res: core.SubmitRejected, Desc: &errMsgPreCommitInfoNotFound}, nil
	}

	if sector.Proof != nil && !hardReset {
		changed := !bytes.Equal(sector.Proof.Proof, info.Proof)

		if changed {
			return core.SubmitProofResp{Res: core.SubmitMismatchedSubmission}, nil
		}

		return core.SubmitProofResp{Res: core.SubmitAccepted}, nil
	}

	sector.Proof = &info

	if err := checkCommit(ctx, *sector, info.Proof, nil, maddr, c.verif, c.stateMgr); err != nil {
		switch err.(type) {
		case *ErrAPI:
			return core.SubmitProofResp{}, err

		case *ErrInvalidDeals,
			*ErrExpiredDeals,
			*ErrNoPrecommit,
			*ErrSectorNumberAllocated,
			*ErrBadSeed,
			*ErrInvalidProof,
			*ErrMarshalAddr:
			errMsg := err.Error()
			return core.SubmitProofResp{Res: core.SubmitRejected, Desc: &errMsg}, nil

		default:
			return core.SubmitProofResp{}, err
		}
	}

	sector.MessageInfo.NeedSend = true
	sector.MessageInfo.CommitCid = nil
	err = c.smgr.Update(ctx, id, core.WorkerOnline, sector.Proof, sector.MessageInfo)
	if err != nil {
		return core.SubmitProofResp{}, err
	}

	go func() {
		c.proPendingChan <- *sector
	}()

	return core.SubmitProofResp{
		Res: core.SubmitAccepted,
	}, nil
}

func (c *CommitmentMgrImpl) ProofState(ctx context.Context, id abi.SectorID) (core.PollProofStateResp, error) {
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return core.PollProofStateResp{}, err
	}

	sector, err := c.smgr.Load(ctx, id, core.WorkerOnline)
	if err != nil {
		return core.PollProofStateResp{}, err
	}

	if sector.MessageInfo.CommitCid == nil {
		if sector.MessageInfo.NeedSend {
			return core.PollProofStateResp{State: core.OnChainStatePending}, err
		}

		return core.PollProofStateResp{State: core.OnChainStateFailed, Desc: &errMsgPublishAttemptFailed}, nil
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.CommitCid.String())
	if err != nil {
		return core.PollProofStateResp{}, err
	}

	mlog := log.With("sector-id", id, "stage", "prove-commit")
	state, maybe := c.handleMessage(ctx, id.Miner, msg, mlog)
	if state == core.OnChainStateLanded {
		si, err := c.stateMgr.StateSectorGetInfo(ctx, maddr, id.Number, nil)

		if err != nil {
			return core.PollProofStateResp{}, err
		}

		if si == nil {
			return core.PollProofStateResp{State: core.OnChainStateShouldAbort, Desc: &errMsgSectorInfoNotFound}, nil
		}
	}

	return core.PollProofStateResp{State: state, Desc: maybe}, nil
}

func (c *CommitmentMgrImpl) SubmitTerminate(ctx context.Context, sid abi.SectorID) (core.SubmitTerminateResp, error) {
	_, height, err := c.stateMgr.ChainHead(ctx)
	if err != nil {
		return core.SubmitTerminateResp{}, err
	}

	_, err = c.terminateSender(sid.Miner)
	if err != nil {
		return core.SubmitTerminateResp{}, err
	}

	// if sector is live, Add to termination queue
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		errMsg := err.Error()
		return core.SubmitTerminateResp{Res: core.SubmitRejected, Desc: &errMsg}, nil
	}

	si, err := c.stateMgr.StateSectorGetInfo(ctx, maddr, sid.Number, nil)
	if err != nil {
		return core.SubmitTerminateResp{}, err
	}
	if si == nil {
		// either already terminated or not committed yet
		pci, err := c.stateMgr.StateSectorPreCommitInfo(ctx, maddr, sid.Number, nil)
		if err != nil {
			return core.SubmitTerminateResp{}, fmt.Errorf("checking precommit presence: %w", err)
		}
		if pci != nil {
			return core.SubmitTerminateResp{}, fmt.Errorf("sector was precommitted but not proven, remove instead of terminating")
		}

		return core.SubmitTerminateResp{}, fmt.Errorf("already terminated")
	}

	// check if maybe already terminated
	loc, err := c.stateMgr.StateSectorPartition(c.ctx, maddr, sid.Number, nil)
	if err != nil {
		return core.SubmitTerminateResp{}, fmt.Errorf("getting location: %w", err)
	}
	if loc == nil {
		return core.SubmitTerminateResp{}, fmt.Errorf("location not found")
	}
	parts, err := c.stateMgr.StateMinerPartitions(c.ctx, maddr, loc.Deadline, nil)
	if err != nil {
		return core.SubmitTerminateResp{}, fmt.Errorf("getting partitions: %w", err)
	}
	live, err := parts[loc.Partition].LiveSectors.IsSet(uint64(sid.Number))
	if err != nil {
		return core.SubmitTerminateResp{}, fmt.Errorf("checking if sector is in live set: %w", err)
	}
	if !live {
		return core.SubmitTerminateResp{}, fmt.Errorf("already terminated")
	}

	loadedSector, err := c.smgr.Load(ctx, sid, core.WorkerOffline)
	if err != nil && !errors.Is(err, kvstore.ErrKeyNotFound) {
		return core.SubmitTerminateResp{}, err
	}

	// check for duplicate actions

	var sector core.SectorState
	newTerminateInfo := core.TerminateInfo{
		TerminatedAt: abi.ChainEpoch(0),
		TerminateCid: nil,
		AddedHeight:  height,
	}

	if err != nil {
		sector.ID = sid
		sector.SectorType = si.SealProof

		ierr := c.smgr.InitWith(ctx, sid, si.SealProof, core.WorkerOffline, newTerminateInfo)
		if ierr != nil {
			return core.SubmitTerminateResp{}, fmt.Errorf("init non-exist snapup sector: %w", err)
		}

	} else {
		if loadedSector.TerminateInfo.AddedHeight > 0 {
			errMsg := "duplicate submit"
			return core.SubmitTerminateResp{Res: core.SubmitDuplicateSubmit, Desc: &errMsg}, nil
		}

		sector = *loadedSector
		err = c.smgr.Update(ctx, sid, core.WorkerOffline, newTerminateInfo)
		if err != nil {
			return core.SubmitTerminateResp{}, err
		}
	}

	sector.TerminateInfo = newTerminateInfo

	go func() {
		c.terminatePendingChan <- sector
	}()

	return core.SubmitTerminateResp{
		Res: core.SubmitAccepted,
	}, nil
}

func (c *CommitmentMgrImpl) TerminateState(ctx context.Context, sid abi.SectorID) (core.TerminateInfo, error) {
	sector, err := c.smgr.Load(ctx, sid, core.WorkerOffline)
	if err != nil {
		return core.TerminateInfo{}, err
	}

	return sector.TerminateInfo, nil
}

func (c *CommitmentMgrImpl) handleMessage(ctx context.Context, mid abi.ActorID, msg *messager.Message, mlog *logging.ZapLogger) (core.OnChainState, *string) {
	mlog = mlog.With("msg-cid", msg.ID, "msg-state", messager.MessageStateToString(msg.State))
	if msg.SignedCid != nil {
		mlog = mlog.With("msg-signed-cid", msg.SignedCid.String())
	}

	mlog.Debug("handle message receipt")

	var maybeMsg *string
	if msg.Receipt != nil && len(msg.Receipt.Return) > 0 {
		msgRet := string(msg.Receipt.Return)
		if msg.State != messager.MessageState.OnChainMsg && msg.State != messager.MessageState.NonceConflictMsg {
			mlog.Warnf("MAYBE WARN from off-chain msg receipt: %s", msgRet)
		}

		maybeMsg = &msgRet
	}

	switch msg.State {
	case messager.MessageState.OnChainMsg, messager.MessageState.NonceConflictMsg:
		confidence := c.cfg.MustMinerConfig(mid).Commitment.Confidence
		if msg.Confidence < confidence {
			return core.OnChainStatePacked, maybeMsg
		}

		if msg.Receipt == nil {
			return core.OnChainStateFailed, &errMsgReceiptNotFound
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			if msg.Receipt.ExitCode == exitcode.SysErrOutOfGas || msg.Receipt.ExitCode == exitcode.ErrInsufficientFunds {
				return core.OnChainStateFailed, maybeMsg
			}

			return core.OnChainStateShouldAbort, maybeMsg
		}

		return core.OnChainStateLanded, maybeMsg

	case messager.MessageState.FailedMsg:
		return core.OnChainStateFailed, maybeMsg

	default:
		return core.OnChainStatePending, maybeMsg
	}
}

var _ core.CommitmentManager = (*CommitmentMgrImpl)(nil)
