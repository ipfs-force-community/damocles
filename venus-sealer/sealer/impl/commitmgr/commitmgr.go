package commitmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/messager"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var log = logging.New("commitmgr")

type CommitmentMgrImpl struct {
	ctx context.Context

	msgClient messager.API

	stateMgr SealingAPI

	smgr api.SectorStateManager

	cfg Cfg

	commitBatcher    map[abi.ActorID]*Batcher
	preCommitBatcher map[abi.ActorID]*Batcher

	prePendingChan chan api.SectorState
	proPendingChan chan api.SectorState

	verif  api.Verifier
	prover api.Prover

	stopOnce sync.Once
	stop     chan struct{}
}

func NewCommitmentMgr(ctx context.Context, commitApi messager.API, stateMgr SealingAPI, smgr api.SectorStateManager,
	cfg *sealer.Config, locker confmgr.RLocker, verif api.Verifier, prover api.Prover,
) (*CommitmentMgrImpl, error) {
	prePendingChan := make(chan api.SectorState, 1024)
	proPendingChan := make(chan api.SectorState, 1024)

	mgr := &CommitmentMgrImpl{
		ctx:       ctx,
		msgClient: commitApi,
		stateMgr:  stateMgr,
		smgr:      smgr,
		cfg: struct {
			*sealer.Config
			confmgr.RLocker
		}{cfg, locker},

		commitBatcher:    map[abi.ActorID]*Batcher{},
		preCommitBatcher: map[abi.ActorID]*Batcher{},

		prePendingChan: prePendingChan,
		proPendingChan: proPendingChan,

		verif:  verif,
		prover: prover,
		stop:   make(chan struct{}),
	}

	go mgr.restartSector(ctx)

	return mgr, nil
}

func (mgr *CommitmentMgrImpl) restartSector(ctx context.Context) {
	sector, err := mgr.smgr.All(ctx)
	if err != nil {
		log.Errorf("load all sector from db failed: %s", err)
		return
	}

	for i := range sector {
		if sector[i].MessageInfo.NeedSend {
			if sector[i].MessageInfo.PreCommitCid == nil {
				mgr.prePendingChan <- *sector[i]
			} else {
				mgr.proPendingChan <- *sector[i]
			}
		}
	}
}

func pushMessage(ctx context.Context, from address.Address, mid abi.ActorID, value abi.TokenAmount, method abi.MethodNum,
	msgClient messager.API, spec messager.MsgMeta, params []byte) (cid.Cid, error) {

	to, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return cid.Undef, err
	}

	msg := venusTypes.UnsignedMessage{
		To:     to,
		From:   from,
		Value:  value,
		Method: method,
		Params: params,
	}
	bk, err := msg.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}
	mb := bk.RawData()
	mcid := cid.Cid{}
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

	return mcid, err
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

func (c *CommitmentMgrImpl) Run() {
	go c.startPreLoop()
	go c.startProLoop()
}

func (c *CommitmentMgrImpl) Stop() {
	log.Info("stop commitment manager")
	c.stopOnce.Do(func() {
		close(c.prePendingChan)
		close(c.proPendingChan)
		close(c.stop)

		for i := range c.commitBatcher {
			c.commitBatcher[i].waitStop()
		}
		for i := range c.commitBatcher {
			c.preCommitBatcher[i].waitStop()
		}
	})
}

func (c *CommitmentMgrImpl) startPreLoop() {
	log.Info("pre commit pending loop start")
	defer log.Info("pre commit pending loop stop")

	for s := range c.prePendingChan {
		miner := s.ID.Miner
		if _, ok := c.commitBatcher[miner]; !ok {
			_, err := address.NewIDAddress(uint64(miner))
			if err != nil {
				log.Error("trans miner from actor to address failed: ", err)
				continue
			}

			ctrl, ok := c.cfg.ctrl(miner)
			if !ok {
				log.Error("no available prove commit control address for %d", miner)
				continue
			}

			c.preCommitBatcher[miner] = NewBatcher(c.ctx, miner, ctrl.PreCommit.Std(), PreCommitProcessor{
				api:       c.stateMgr,
				msgClient: c.msgClient,
				smgr:      c.smgr,
				config:    c.cfg,
			})
		}

		c.preCommitBatcher[miner].Add(s)
	}
}

func (c *CommitmentMgrImpl) startProLoop() {
	log.Info("prove commit pending loop start")
	defer log.Info("prove commit pending loop stop")

	for s := range c.proPendingChan {
		miner := s.ID.Miner
		if _, ok := c.commitBatcher[miner]; !ok {
			_, err := address.NewIDAddress(uint64(miner))
			if err != nil {
				log.Error("trans miner from actor %d to address failed: ", miner, err)
				continue
			}

			ctrl, ok := c.cfg.ctrl(miner)
			if !ok {
				log.Error("no available prove commit control address for %d", miner)
				continue
			}

			c.commitBatcher[miner] = NewBatcher(c.ctx, miner, ctrl.ProveCommit.Std(), CommitProcessor{
				api:       c.stateMgr,
				msgClient: c.msgClient,
				smgr:      c.smgr,
				config:    c.cfg,
				prover:    c.prover,
			})
		}
		c.commitBatcher[miner].Add(s)
	}
}

func (c *CommitmentMgrImpl) SubmitPreCommit(ctx context.Context, id abi.SectorID, info api.PreCommitInfo, hardReset bool) (api.SubmitPreCommitResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitInvalidInfo}, err
	}

	if hardReset {
		log.Infof("sector %d is hard reset will skip all check in precommit", id)
		sector.MessageInfo.NeedSend = true
		sector.MessageInfo.PreCommitCid = nil
		sector.Pre = &info

		go func() {
			c.prePendingChan <- *sector
		}()
		goto Commit
	}

	if sector.Pre != nil {
		if sector.Pre.CommD != info.CommD {
			return api.SubmitPreCommitResp{Res: api.SubmitMismatchedSubmission}, nil
		}

		if sector.Pre.CommR != info.CommR {
			return api.SubmitPreCommitResp{Res: api.SubmitMismatchedSubmission}, nil
		}

		if sector.Pre.Ticket.Epoch != info.Ticket.Epoch || !bytes.Equal(sector.Pre.Ticket.Ticket, info.Ticket.Ticket) {
			return api.SubmitPreCommitResp{Res: api.SubmitMismatchedSubmission}, nil
		}
	}

	sector.Pre = &info

	err = checkPrecommit(ctx, maddr, *sector, c.stateMgr)

	if _, ok := err.(ErrPrecommitOnChain); ok {
		return api.SubmitPreCommitResp{Res: api.SubmitAccepted}, nil
	}

	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}

	if sector.MessageInfo.NeedSend == false && sector.MessageInfo.PreCommitCid == nil {
		sector.MessageInfo.NeedSend = true
		sector.MessageInfo.PreCommitCid = nil

		go func() {
			c.prePendingChan <- *sector
		}()
	}
Commit:
	err = c.smgr.Update(ctx, sector.ID, sector.Pre, &sector.MessageInfo)
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}
	log.Infof("submit sector %d precommit success", id)
	return api.SubmitPreCommitResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c *CommitmentMgrImpl) PreCommitState(ctx context.Context, id abi.SectorID) (api.PollPreCommitStateResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.PollPreCommitStateResp{}, err
	}
	// pending
	if sector.MessageInfo.PreCommitCid == nil && sector.MessageInfo.NeedSend {
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, nil
	}
	// failed
	if sector.MessageInfo.PreCommitCid == nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, nil
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.PreCommitCid.String())
	if err != nil {
		return api.PollPreCommitStateResp{}, err
	}
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.PollPreCommitStateResp{}, err
	}

	switch msg.State {
	case messager.OnChainMsg:
		confidence := c.cfg.policy(id.Miner).MsgConfidence
		if msg.Confidence < confidence {
			return api.PollPreCommitStateResp{State: api.OnChainStatePacked}, nil
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, nil
		}
		_, err := c.stateMgr.StateSectorPreCommitInfo(ctx, maddr, id.Number, nil)
		if err == ErrSectorAllocated {
			return api.PollPreCommitStateResp{State: api.OnChainStatePermFailed}, nil
		}

		if err != nil {
			return api.PollPreCommitStateResp{}, err
		}

		return api.PollPreCommitStateResp{State: api.OnChainStateLanded}, nil
	case messager.FailedMsg:
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, nil
	default:
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, nil
	}
}

func (c *CommitmentMgrImpl) SubmitProof(ctx context.Context, id abi.SectorID, info api.ProofInfo, hardReset bool) (api.SubmitProofResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.SubmitProofResp{}, err
	}
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitInvalidInfo}, nil
	}

	if hardReset {
		log.Infof("sector %d is hard reset will skip all check in prove commit", id)
		sector.MessageInfo.NeedSend = true
		sector.MessageInfo.CommitCid = nil

		sector.Proof = &info
		go func() {
			c.proPendingChan <- *sector
		}()

		goto Commit
	}

	if sector.Pre == nil {
		return api.SubmitProofResp{Res: api.SubmitRejected}, nil
	}

	if !hardReset && sector.Proof != nil && !bytes.Equal(sector.Proof.Proof, info.Proof) {
		return api.SubmitProofResp{Res: api.SubmitMismatchedSubmission}, nil
	}

	sector.Proof = &info

	if err := checkCommit(ctx, *sector, info.Proof, nil, maddr, c.verif, c.stateMgr); err != nil {
		switch err.(type) {
		case ErrInvalidDeals, ErrExpiredDeals, ErrNoPrecommit, ErrSectorNumberAllocated:
			return api.SubmitProofResp{Res: api.SubmitRejected}, nil
		case ErrBadSeed, ErrInvalidProof, ErrMarshalAddr:
			return api.SubmitProofResp{Res: api.SubmitInvalidInfo}, nil
		default:
			return api.SubmitProofResp{}, err
		}
	}

	if sector.MessageInfo.NeedSend == false && sector.MessageInfo.CommitCid == nil {
		sector.MessageInfo.NeedSend = true
		sector.MessageInfo.CommitCid = nil

		go func() {
			c.proPendingChan <- *sector
		}()
	}

Commit:
	err = c.smgr.Update(ctx, id, sector.Proof, &sector.MessageInfo)
	if err != nil {
		return api.SubmitProofResp{}, err
	}
	log.Infof("submit sector %d prove commit success", id)
	return api.SubmitProofResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c *CommitmentMgrImpl) ProofState(ctx context.Context, id abi.SectorID) (api.PollProofStateResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.PollProofStateResp{}, err
	}

	if sector.MessageInfo.CommitCid == nil && sector.MessageInfo.NeedSend {
		return api.PollProofStateResp{State: api.OnChainStatePending}, err
	}
	if sector.MessageInfo.CommitCid == nil {
		return api.PollProofStateResp{State: api.OnChainStateFailed}, err
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.CommitCid.String())
	if err != nil {
		return api.PollProofStateResp{}, err
	}
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.PollProofStateResp{}, err
	}

	switch msg.State {
	case messager.OnChainMsg:
		confidence := c.cfg.policy(id.Miner).MsgConfidence
		if msg.Confidence < confidence {
			return api.PollProofStateResp{State: api.OnChainStatePacked}, nil
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			return api.PollProofStateResp{State: api.OnChainStateFailed}, nil
		}
		si, err := c.stateMgr.StateSectorGetInfo(ctx, maddr, id.Number, nil)

		if err != nil {
			return api.PollProofStateResp{}, err
		}

		if si != nil {
			return api.PollProofStateResp{State: api.OnChainStateLanded}, nil
		}

		return api.PollProofStateResp{State: api.OnChainStateFailed}, nil
	case messager.FailedMsg:
		return api.PollProofStateResp{State: api.OnChainStateFailed}, err
	default:
		return api.PollProofStateResp{State: api.OnChainStatePending}, err
	}
}

var _ api.CommitmentManager = (*CommitmentMgrImpl)(nil)
