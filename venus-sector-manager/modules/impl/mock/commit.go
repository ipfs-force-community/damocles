package mock

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

var _ core.CommitmentManager = (*commitMgr)(nil)

func NewCommitManager() core.CommitmentManager {
	cmgr := &commitMgr{}

	cmgr.pres.commits = map[abi.SectorID]core.PreCommitInfo{}
	cmgr.proofs.proofs = map[abi.SectorID]core.ProofInfo{}
	return cmgr
}

type commitMgr struct {
	pres struct {
		sync.RWMutex
		commits map[abi.SectorID]core.PreCommitInfo
	}

	proofs struct {
		sync.RWMutex
		proofs map[abi.SectorID]core.ProofInfo
	}

	terminates struct {
		sync.RWMutex
		terminates map[abi.SectorID]struct{}
	}
}

func (c *commitMgr) SubmitPreCommit(ctx context.Context, sid abi.SectorID, info core.PreCommitInfo, hardReset bool) (core.SubmitPreCommitResp, error) {
	c.pres.Lock()
	defer c.pres.Unlock()

	if !hardReset {
		if _, ok := c.pres.commits[sid]; ok {
			return core.SubmitPreCommitResp{
				Res:  core.SubmitDuplicateSubmit,
				Desc: nil,
			}, nil
		}
	}

	c.pres.commits[sid] = info

	return core.SubmitPreCommitResp{
		Res:  core.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (c *commitMgr) PreCommitState(ctx context.Context, sid abi.SectorID) (core.PollPreCommitStateResp, error) {
	c.pres.RLock()
	defer c.pres.RUnlock()

	if _, ok := c.pres.commits[sid]; ok {
		return core.PollPreCommitStateResp{
			State: core.OnChainStateLanded,
			Desc:  nil,
		}, nil
	}

	return core.PollPreCommitStateResp{
		State: core.OnChainStateNotFound,
		Desc:  nil,
	}, nil
}

func (c *commitMgr) SubmitProof(ctx context.Context, sid abi.SectorID, info core.ProofInfo, hardReset bool) (core.SubmitProofResp, error) {
	c.proofs.Lock()
	defer c.proofs.Unlock()

	if !hardReset {
		if _, ok := c.proofs.proofs[sid]; ok {
			return core.SubmitProofResp{
				Res:  core.SubmitDuplicateSubmit,
				Desc: nil,
			}, nil
		}
	}

	c.proofs.proofs[sid] = info

	return core.SubmitProofResp{
		Res:  core.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (c *commitMgr) ProofState(ctx context.Context, sid abi.SectorID) (core.PollProofStateResp, error) {
	c.proofs.RLock()
	defer c.proofs.RUnlock()

	if _, ok := c.proofs.proofs[sid]; ok {
		return core.PollProofStateResp{
			State: core.OnChainStateLanded,
			Desc:  nil,
		}, nil
	}

	return core.PollProofStateResp{
		State: core.OnChainStateNotFound,
		Desc:  nil,
	}, nil
}

func (c *commitMgr) SubmitTerminate(ctx context.Context, sid abi.SectorID) (core.SubmitTerminateResp, error) {
	c.terminates.Lock()
	defer c.terminates.Unlock()

	if _, ok := c.terminates.terminates[sid]; ok {
		return core.SubmitTerminateResp{
			Res:  core.SubmitDuplicateSubmit,
			Desc: nil,
		}, nil
	}

	c.terminates.terminates[sid] = struct{}{}

	return core.SubmitTerminateResp{
		Res:  core.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (c *commitMgr) TerminateState(ctx context.Context, sid abi.SectorID) (core.TerminateInfo, error) {
	return core.TerminateInfo{}, nil
}
