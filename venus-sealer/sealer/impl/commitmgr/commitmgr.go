package commitmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	messager "github.com/filecoin-project/venus-messager/types"
	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var log = logging.New("commitmgr")

type CommitmentMgrImpl struct {
	ctx context.Context

	msgClient venusMessager.IMessager

	stateMgr SealingAPI

	smgr api.SectorStateManager

	cfg struct {
		*sealer.Config
		confmgr.RLocker
	}

	commitBatcher    map[abi.ActorID]*Batcher
	preCommitBatcher map[abi.ActorID]*Batcher

	prePendingChan chan api.SectorState
	proPendingChan chan api.SectorState

	verif  api.Verifier
	prover api.Prover

	stop chan struct{}
}

func NewCommitmentMgr(ctx context.Context, commitApi venusMessager.IMessager, stateMgr SealingAPI, smgr api.SectorStateManager,
	cfg *sealer.Config, locker confmgr.RLocker, verif api.Verifier, prover api.Prover,
) (*CommitmentMgrImpl, error) {
	pendingChan := make(chan api.SectorState, 1024)
	prePendingChan := make(chan api.SectorState, 1024)

	mgr := CommitmentMgrImpl{
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

		proPendingChan: pendingChan,
		verif:          verif,
		prover:         prover,
		stop:           make(chan struct{}),
	}

	mgr.Run()

	go func() {
		select {
		case <-ctx.Done():
			close(pendingChan)
			close(prePendingChan)
		}

		for i := range mgr.commitBatcher {
			mgr.commitBatcher[i].waitStop()
		}
		for i := range mgr.commitBatcher {
			mgr.preCommitBatcher[i].waitStop()
		}

		close(mgr.stop)
	}()

	return &mgr, nil
}

func getPreCommitControlAddress(miner address.Address, cfg struct {
	*sealer.Config
	confmgr.RLocker
}) (address.Address, error) {
	cfg.Lock()
	defer cfg.Unlock()
	return address.NewFromString(cfg.CommitmentManager[miner].PreCommitControlAddress)
}

func getProCommitControlAddress(miner address.Address, cfg struct {
	*sealer.Config
	confmgr.RLocker
}) (address.Address, error) {
	cfg.Lock()
	defer cfg.Unlock()

	return address.NewFromString(cfg.CommitmentManager[miner].ProCommitControlAddress)
}

func pushMessage(ctx context.Context, from, to address.Address, value abi.TokenAmount, method abi.MethodNum,
	msgClient venusMessager.IMessager, spec messager.MsgMeta, params []byte) (cid.Cid, error) {
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
		return cid.Undef, xerrors.Errorf("push message with id failed: %w", err)
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
	{
		go func() {
			for s := range c.prePendingChan {
				miner := s.ID.Miner
				if _, ok := c.commitBatcher[miner]; !ok {
					maddr, err := address.NewIDAddress(uint64(miner))
					if err != nil {
						log.Error("trans miner from actor to address failed: ", err)
						continue
					}
					c.preCommitBatcher[miner] = NewBatcher(c.ctx, maddr, PreCommitProcessor{
						api:       c.stateMgr,
						msgClient: c.msgClient,
						smgr:      c.smgr,
						config:    c.cfg,
					})
				}

				c.preCommitBatcher[miner].Add(s)
			}
		}()
	}

	{
		go func() {
			for s := range c.proPendingChan {
				miner := s.ID.Miner
				if _, ok := c.commitBatcher[miner]; !ok {
					maddr, err := address.NewIDAddress(uint64(miner))
					if err != nil {
						log.Error("trans miner from actor to address failed: ", err)
						continue
					}

					c.commitBatcher[miner] = NewBatcher(c.ctx, maddr, CommitProcessor{
						api:       c.stateMgr,
						msgClient: c.msgClient,
						smgr:      c.smgr,
						config:    c.cfg,
						prover:    c.prover,
					})
				}
				c.commitBatcher[miner].Add(s)
			}
		}()
	}
	return
}

func (c CommitmentMgrImpl) SubmitPreCommit(ctx context.Context, id abi.SectorID, info api.PreCommitInfo, reset bool) (api.SubmitPreCommitResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
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

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
	}
	err = checkPrecommit(ctx, maddr, *sector, c.stateMgr)

	if _, ok := err.(ErrPrecommitOnChain); ok {
		return api.SubmitPreCommitResp{Res: api.SubmitDuplicateSubmit}, err
	}

	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitRejected}, err
	}

	if sector.MessageInfo != nil && sector.MessageInfo.PreCommitCid != nil {
		msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.PreCommitCid.String())
		if err != nil {
			return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
		}
		switch msg.State {
		case messager.OnChainMsg:
			c.cfg.Lock()
			confidence := c.cfg.CommitmentManager[maddr].MsgConfidence
			c.cfg.Unlock()
			if msg.Confidence < confidence {
				return api.SubmitPreCommitResp{Res: api.SubmitDuplicateSubmit}, err
			}
			if msg.Receipt.ExitCode != exitcode.Ok {
				break
			}
			pci, err := c.stateMgr.StateSectorPreCommitInfo(ctx, maddr, id.Number, nil)
			if err == ErrSectorAllocated {
				return api.SubmitPreCommitResp{Res: api.SubmitRejected}, err
			}
			if err != nil {
				return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
			}

			if pci != nil {
				return api.SubmitPreCommitResp{Res: api.SubmitDuplicateSubmit}, err
			}
			log.Infof("get sector %d info return nil err and nil pci, that's wired!!!", sector.ID.Number)
		case messager.FailedMsg:
			// same reason and threaten as prove commit
		default:
			return api.SubmitPreCommitResp{Res: api.Submitted}, err
		}
	}

	err = c.smgr.Update(ctx, sector.ID, sector.Pre)
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}
	go func() {
		c.prePendingChan <- *sector
	}()
	return api.SubmitPreCommitResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c CommitmentMgrImpl) PreCommitState(ctx context.Context, id abi.SectorID) (api.PollPreCommitStateResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
	}
	// pending
	if sector.MessageInfo == nil || (sector.MessageInfo.PreCommitCid == nil && sector.MessageInfo.NeedSend) {
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, nil
	}
	// failed
	if sector.MessageInfo.PreCommitCid == nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, nil
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.PreCommitCid.String())
	if err != nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
	}
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
	}

	switch msg.State {
	case messager.OnChainMsg:
		c.cfg.Lock()
		confidence := c.cfg.CommitmentManager[maddr].MsgConfidence
		c.cfg.Unlock()
		if msg.Confidence < confidence {
			return api.PollPreCommitStateResp{State: api.OnChainStatePending}, err
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, nil
		}
		pci, err := c.stateMgr.StateSectorPreCommitInfo(ctx, maddr, id.Number, nil)
		if err == ErrSectorAllocated {
			return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, err
		}
		if err != nil {
			return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
		}

		if pci != nil {
			return api.PollPreCommitStateResp{State: api.OnChainStateLanded}, nil
		}
	case messager.FailedMsg:
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, err
	default:
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, err
	}
	return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
}

func (c CommitmentMgrImpl) SubmitProof(ctx context.Context, id abi.SectorID, info api.ProofInfo, reset bool) (api.SubmitProofResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitUnknown}, err
	}

	if sector.Pre == nil || sector.MessageInfo == nil {
		return api.SubmitProofResp{Res: api.SubmitRejected}, fmt.Errorf("miss pre info in sector state")
	}

	if sector.Proof != nil && !bytes.Equal(sector.Proof.Proof, info.Proof) {
		return api.SubmitProofResp{Res: api.SubmitMismatchedSubmission}, err
	}

	sector.Proof = &info
	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitUnknown}, err
	}
	err = checkCommit(ctx, *sector, info.Proof, nil, maddr, c.verif, c.stateMgr)
	// TODO: err can retry and err can not retry
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitRejected}, err
	}

	if sector.MessageInfo.CommitCid != nil {
		msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.CommitCid.String())
		if err != nil {
			return api.SubmitProofResp{Res: api.SubmitUnknown}, err
		}
		switch msg.State {
		case messager.OnChainMsg:
			c.cfg.Lock()
			confidence := c.cfg.CommitmentManager[maddr].MsgConfidence
			c.cfg.Unlock()

			if msg.Confidence < confidence {
				return api.SubmitProofResp{Res: api.SubmitDuplicateSubmit}, err
			}

			if msg.Receipt.ExitCode != exitcode.Ok {
				break
			}
			s, err := c.stateMgr.StateSectorGetInfo(ctx, maddr, id.Number, nil)
			if err != nil {
				return api.SubmitProofResp{Res: api.SubmitUnknown}, err
			}

			if s != nil {
				return api.SubmitProofResp{Res: api.SubmitDuplicateSubmit}, err
			}
			log.Infof("no proof found after cron, will retry send msg of sector %d", sector.ID.Number)
		case messager.FailedMsg:
			// this mostly cause by estimate fail
			// since sector pass validate check, let it retry is fine
		default:
			return api.SubmitProofResp{Res: api.SubmitDuplicateSubmit}, err
		}
	}
	sector.MessageInfo.NeedSend = true
	sector.MessageInfo.CommitCid = nil

	err = c.smgr.Update(ctx, id, sector.Proof, sector.MessageInfo)
	if err != nil {
		return api.SubmitProofResp{}, err
	}
	go func() {
		c.proPendingChan <- *sector
	}()
	return api.SubmitProofResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c CommitmentMgrImpl) ProofState(ctx context.Context, id abi.SectorID) (api.PollProofStateResp, error) {
	sector, err := c.smgr.Load(ctx, id)
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}
	if sector.MessageInfo == nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, fmt.Errorf("miss pre info in sector state")
	}
	// pending
	if sector.MessageInfo.CommitCid == nil && sector.MessageInfo.NeedSend {
		return api.PollProofStateResp{State: api.OnChainStatePending}, err
	}
	// unknown state
	if sector.MessageInfo.CommitCid == nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.MessageInfo.CommitCid.String())
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}
	switch msg.State {
	case messager.OnChainMsg:
		c.cfg.Lock()
		confidence := c.cfg.CommitmentManager[maddr].MsgConfidence
		c.cfg.Unlock()
		if msg.Confidence < confidence {
			return api.PollProofStateResp{State: api.OnChainStatePending}, err
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			return api.PollProofStateResp{State: api.OnChainStateFailed}, nil
		}
		pci, err := c.stateMgr.StateSectorGetInfo(ctx, maddr, id.Number, nil)

		if err != nil {
			return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
		}

		if pci != nil {
			return api.PollProofStateResp{State: api.OnChainStateLanded}, err
		}
	case messager.FailedMsg:
		return api.PollProofStateResp{State: api.OnChainStateFailed}, err
	default:
		return api.PollProofStateResp{State: api.OnChainStatePending}, err
	}
	return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
}

var _ api.CommitmentManager = (*CommitmentMgrImpl)(nil)
