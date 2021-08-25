package mock

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/sealer/api"
)

var _ api.CommitmentManager = (*commitMgr)(nil)

func NewCommitManager() api.CommitmentManager {
	cmgr := &commitMgr{}

	cmgr.pres.commits = map[abi.SectorID]api.PreCommitInfo{}
	cmgr.proofs.proofs = map[abi.SectorID]api.ProofInfo{}
	return cmgr
}

type commitMgr struct {
	pres struct {
		sync.RWMutex
		commits map[abi.SectorID]api.PreCommitInfo
	}

	proofs struct {
		sync.RWMutex
		proofs map[abi.SectorID]api.ProofInfo
	}
}

func (c *commitMgr) SubmitPreCommit(ctx context.Context, sid abi.SectorID, info api.PreCommitInfo, hardReset bool) (api.SubmitPreCommitResp, error) {
	c.pres.Lock()
	defer c.pres.Unlock()

	if !hardReset {
		if _, ok := c.pres.commits[sid]; ok {
			return api.SubmitPreCommitResp{
				Res:  api.SubmitDuplicateSubmit,
				Desc: nil,
			}, nil
		}
	}

	c.pres.commits[sid] = info

	return api.SubmitPreCommitResp{
		Res:  api.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (c *commitMgr) PreCommitState(ctx context.Context, sid abi.SectorID) (api.PollPreCommitStateResp, error) {
	c.pres.RLock()
	defer c.pres.RUnlock()

	if _, ok := c.pres.commits[sid]; ok {
		return api.PollPreCommitStateResp{
			State: api.OnChainStateLanded,
			Desc:  nil,
		}, nil
	}

	return api.PollPreCommitStateResp{
		State: api.OnChainStateNotFound,
		Desc:  nil,
	}, nil
}

func (c *commitMgr) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofInfo, hardReset bool) (api.SubmitProofResp, error) {
	c.proofs.Lock()
	defer c.proofs.Unlock()

	if !hardReset {
		if _, ok := c.proofs.proofs[sid]; ok {
			return api.SubmitProofResp{
				Res:  api.SubmitDuplicateSubmit,
				Desc: nil,
			}, nil
		}
	}

	c.proofs.proofs[sid] = info

	return api.SubmitProofResp{
		Res:  api.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (c *commitMgr) ProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	c.proofs.RLock()
	defer c.proofs.RUnlock()

	if _, ok := c.proofs.proofs[sid]; ok {
		return api.PollProofStateResp{
			State: api.OnChainStateLanded,
			Desc:  nil,
		}, nil
	}

	return api.PollProofStateResp{
		State: api.OnChainStateNotFound,
		Desc:  nil,
	}, nil
}
