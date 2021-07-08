package commitmgr

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
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

	ds api.SectorsDatastore

	cfg struct {
		*sealer.Config
		confmgr.RLocker
	}

	commitBatcher map[abi.ActorID]*CommitBatcher
	CommitBatcherCtor
	preCommitBatcher map[abi.ActorID]*PreCommitBatcher
	PreCommitBatcherCtor

	prePendingChan chan abi.SectorID
	proPendingChan chan abi.SectorID

	verif  Verifier
	prover Prover

	stop chan struct{}
}

func NewCommitmentMgr(ctx context.Context, commitApi venusMessager.IMessager, stateMgr SealingAPI, ds api.SectorsDatastore,
	cfg *sealer.Config, locker confmgr.RLocker, verif Verifier, prover Prover,
) (*CommitmentMgrImpl, error) {
	pendingChan := make(chan abi.SectorID, 1024)
	prePendingChan := make(chan abi.SectorID, 1024)
	mgr := CommitmentMgrImpl{
		ctx:       ctx,
		msgClient: commitApi,
		stateMgr:  stateMgr,
		ds:        ds,
		cfg: struct {
			*sealer.Config
			confmgr.RLocker
		}{cfg, locker},

		commitBatcher:        map[abi.ActorID]*CommitBatcher{},
		CommitBatcherCtor:    NewCommitBatcher,
		preCommitBatcher:     map[abi.ActorID]*PreCommitBatcher{},
		PreCommitBatcherCtor: NewPreCommitBatcher,

		prePendingChan: prePendingChan,

		proPendingChan: pendingChan,
		verif:          verif,
		prover:         prover,
		stop:           make(chan struct{}),
	}
	ch := mgr.Run(ctx)
	go func() {
		select {
		case <-ctx.Done():
			close(pendingChan)
			close(prePendingChan)
		}
		<-ch
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
	control, ok := cfg.CommitmentManager.PreCommitControlAddress[miner.String()]
	cfg.Unlock()
	if !ok {
		return address.Undef, xerrors.Errorf("no preCommit control address of Miner %s in config file, this is not approved", miner.String())
	}

	return address.NewFromString(control)
}

func getProCommitControlAddress(miner address.Address, cfg struct {
	*sealer.Config
	confmgr.RLocker
}) (address.Address, error) {
	cfg.Lock()
	control, ok := cfg.CommitmentManager.ProCommitControlAddress[miner.String()]
	cfg.Unlock()
	if !ok {
		return address.Undef, xerrors.Errorf("no proCommit control address of Miner %s in config file, this is not approved", miner.String())
	}

	return address.NewFromString(control)
}

func (c *CommitmentMgrImpl) pushPreCommitSingle(ctx context.Context, s abi.SectorID) error {
	to, err := address.NewIDAddress(uint64(s.Miner))
	if err != nil {
		return xerrors.Errorf("marshal into to failed %w", err)
	}
	from, err := getPreCommitControlAddress(to, c.cfg)
	if err != nil {
		return err
	}
	var spec messager.MsgMeta
	c.cfg.Lock()
	spec.GasOverEstimation = c.cfg.CommitmentManager.PreCommitGasOverEstimation
	spec.MaxFeeCap = c.cfg.CommitmentManager.MaxPreCommitFeeCap
	c.cfg.Unlock()

	return pushPreCommitSingle(ctx, c.ds, c.msgClient, from, s, spec, c.stateMgr)
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

func getMsgMeta(cfg struct {
	*sealer.Config
	confmgr.RLocker
}) messager.MsgMeta {
	var spec messager.MsgMeta
	cfg.Lock()
	spec.GasOverEstimation = cfg.CommitmentManager.ProCommitGasOverEstimation
	spec.MaxFeeCap = cfg.CommitmentManager.MaxProCommitFeeCap
	cfg.Unlock()
	return spec
}

func (c *CommitmentMgrImpl) pushCommitSingle(ctx context.Context, s abi.SectorID) error {
	to, err := address.NewIDAddress(uint64(s.Miner))
	if err != nil {
		return xerrors.Errorf("marshal into to failed %w", err)
	}
	from, err := getProCommitControlAddress(to, c.cfg)
	if err != nil {
		return err
	}
	spec := getMsgMeta(c.cfg)
	return pushCommitSingle(ctx, c.ds, c.msgClient, from, s, spec, c.stateMgr)
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

func (c *CommitmentMgrImpl) Run(ctx context.Context) <-chan struct{} {
	wg := sync.WaitGroup{}
	ch := make(chan struct{})
	{
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range c.prePendingChan {
				c.cfg.Lock()
				individual := c.cfg.CommitmentManager.EnableBatchPreCommit
				c.cfg.Unlock()
				if individual {
					err := c.pushPreCommitSingle(c.ctx, s)
					if err != nil {
						log.Error(err)
					}
					continue
				}

				err := c.preCommitBatcher[s.Miner].Add(c.ctx, s)
				if err != nil {
					log.Errorf("add %d %s into pre commit batch pool meet error %s", s.Number, s.Miner, err.Error())
				}
			}
		}()
	}

	{
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range c.proPendingChan {
				c.cfg.Lock()
				individual := c.cfg.CommitmentManager.EnableBatchProCommit
				c.cfg.Unlock()
				if individual {
					err := c.pushCommitSingle(c.ctx, s)
					if err != nil {
						log.Error(err)
					}
					continue
				}

				err := c.commitBatcher[s.Miner].Add(c.ctx, s)
				if err != nil {
					log.Errorf("add %d %s into commit batch pool meet error %s", s.Number, s.Miner, err.Error())
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

func (c CommitmentMgrImpl) SubmitPreCommit(ctx context.Context, id abi.SectorID, info api.PreCommitOnChainInfo) (api.SubmitPreCommitResp, error) {
	// Can these codes move to lower bound?
	select {
	case <-c.ctx.Done():
		return api.SubmitPreCommitResp{}, xerrors.Errorf("server is closing!")
	default:
	}

	sector, err := c.ds.GetSector(ctx, id)
	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
	}
	commR, err := commcid.ReplicaCommitmentV1ToCID(info.CommR[:])
	if err != nil {
		return api.SubmitPreCommitResp{}, xerrors.Errorf("CommR invalid %w", err)
	}
	commD, err := commcid.ReplicaCommitmentV1ToCID(info.CommD[:])
	if err != nil {
		return api.SubmitPreCommitResp{}, xerrors.Errorf("CommD invalid %w", err)
	}

	if sector.CommD != nil && *sector.CommD != commD {
		return api.SubmitPreCommitResp{Res: api.SubmitMismatchedSubmission}, nil
	}

	if sector.CommR != nil && *sector.CommR != commR {
		return api.SubmitPreCommitResp{Res: api.SubmitMismatchedSubmission}, nil
	}

	sector.CommR = &commR
	sector.NeedSend = true

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
	}
	err = checkPrecommit(ctx, maddr, sector, c.stateMgr)

	if _, ok := err.(ErrPrecommitOnChain); ok {
		return api.SubmitPreCommitResp{Res: api.SubmitDuplicateSubmit}, err
	}

	if err != nil {
		return api.SubmitPreCommitResp{Res: api.SubmitRejected}, err
	}

	if sector.PreCommitCid != nil {
		msg, err := c.msgClient.GetMessageByUid(ctx, sector.PreCommitCid.String())
		if err != nil {
			return api.SubmitPreCommitResp{Res: api.SubmitUnknown}, err
		}
		switch msg.State {
		case messager.OnChainMsg:
			c.cfg.Lock()
			confidence := c.cfg.CommitmentManager.MsgConfidence
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
			log.Infof("get sector %d info return nil err and nil pci, that's wired!!!", sector.SectorID.Number)
		case messager.FailedMsg:
			// same reason and threaten as prove commit
		default:
			return api.SubmitPreCommitResp{Res: api.Submitted}, err
		}
	}

	err = c.ds.PutSector(ctx, sector)
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}
	go func() {
		c.prePendingChan <- id
	}()
	return api.SubmitPreCommitResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c CommitmentMgrImpl) PreCommitState(ctx context.Context, id abi.SectorID) (api.PollPreCommitStateResp, error) {
	select {
	case <-c.ctx.Done():
		return api.PollPreCommitStateResp{}, xerrors.Errorf("server is closing!")
	default:
	}

	sector, err := c.ds.GetSector(ctx, id)
	if err != nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
	}
	// pending
	if sector.PreCommitCid == nil && sector.NeedSend {
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, err
	}
	// failed
	if sector.PreCommitCid == nil {
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, err
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.PreCommitCid.String())
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
		confidence := c.cfg.CommitmentManager.MsgConfidence
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
			return api.PollPreCommitStateResp{State: api.OnChainStateLanded}, err
		}
	case messager.FailedMsg:
		return api.PollPreCommitStateResp{State: api.OnChainStateFailed}, err
	default:
		return api.PollPreCommitStateResp{State: api.OnChainStatePending}, err
	}
	return api.PollPreCommitStateResp{State: api.OnChainStateUnknown}, err
}

func (c CommitmentMgrImpl) SubmitProof(ctx context.Context, id abi.SectorID, info api.ProofOnChainInfo) (api.SubmitProofResp, error) {
	select {
	case <-c.ctx.Done():
		return api.SubmitProofResp{}, xerrors.Errorf("server is closing!")
	default:
	}

	sector, err := c.ds.GetSector(ctx, id)
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitUnknown}, err
	}
	if len(sector.Proof) != 0 && !bytes.Equal(sector.Proof, info.Proof) {
		return api.SubmitProofResp{Res: api.SubmitMismatchedSubmission}, err
	}

	sector.Proof = info.Proof
	sector.NeedSend = true

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitUnknown}, err
	}
	err = checkCommit(ctx, sector, info.Proof, nil, maddr, c.verif, c.stateMgr)
	// TODO: err can retry and err can not retry
	if err != nil {
		return api.SubmitProofResp{Res: api.SubmitRejected}, err
	}

	if sector.CommitCid != nil {
		msg, err := c.msgClient.GetMessageByUid(ctx, sector.CommitCid.String())
		if err != nil {
			return api.SubmitProofResp{Res: api.SubmitUnknown}, err
		}
		switch msg.State {
		case messager.OnChainMsg:
			c.cfg.Lock()
			confidence := c.cfg.CommitmentManager.MsgConfidence
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
			log.Infof("no proof found after cron, will retry send msg of sector %d", sector.SectorID.Number)
		case messager.FailedMsg:
			// this mostly cause by estimate fail
			// since sector pass validate check, let it retry is fine
		default:
			return api.SubmitProofResp{Res: api.SubmitDuplicateSubmit}, err
		}
	}

	err = c.ds.PutSector(ctx, sector)
	if err != nil {
		return api.SubmitProofResp{}, err
	}
	go func() {
		c.proPendingChan <- id
	}()
	return api.SubmitProofResp{
		Res: api.SubmitAccepted,
	}, nil
}

func (c CommitmentMgrImpl) ProofState(ctx context.Context, id abi.SectorID) (api.PollProofStateResp, error) {
	select {
	case <-c.ctx.Done():
		return api.PollProofStateResp{}, xerrors.Errorf("server is closing!")
	default:
	}

	sector, err := c.ds.GetSector(ctx, id)
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}

	// pending
	if sector.CommitCid == nil && sector.NeedSend {
		return api.PollProofStateResp{State: api.OnChainStatePending}, err
	}
	// unknown state
	if sector.CommitCid == nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}

	maddr, err := address.NewIDAddress(uint64(id.Miner))
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}

	msg, err := c.msgClient.GetMessageByUid(ctx, sector.CommitCid.String())
	if err != nil {
		return api.PollProofStateResp{State: api.OnChainStateUnknown}, err
	}
	switch msg.State {
	case messager.OnChainMsg:
		c.cfg.Lock()
		confidence := c.cfg.CommitmentManager.MsgConfidence
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
