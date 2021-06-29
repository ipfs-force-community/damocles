package impl

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/sealing/api"
)

var _ api.SealerAPI = (*Mock)(nil)

type Mock struct {
	miner        abi.ActorID
	sectorNumber uint64
	proofType    abi.RegisteredSealProof
	ticket       api.Ticket
	seed         api.Seed

	preCommits struct {
		sync.RWMutex
		commits map[abi.SectorID]api.PreCommitOnChainInfo
	}

	proofs struct {
		sync.RWMutex
		proofs map[abi.SectorID]api.ProofOnChainInfo
	}
}

func (m *Mock) AllocateSector(ctx context.Context, spec api.AllocateSectorSpec) (*api.AllocatedSector, error) {
	minerFits := len(spec.AllowedMiners) == 0
	for _, want := range spec.AllowedMiners {
		if want == m.miner {
			minerFits = true
			break
		}
	}

	if !minerFits {
		return nil, nil
	}

	typeFits := len(spec.AllowedProofTypes) == 0
	for _, typ := range spec.AllowedProofTypes {
		if typ == m.proofType {
			typeFits = true
			break
		}
	}

	if !typeFits {
		return nil, nil
	}

	next := atomic.AddUint64(&m.sectorNumber, 1)
	return &api.AllocatedSector{
		ID: abi.SectorID{
			Miner:  m.miner,
			Number: abi.SectorNumber(next),
		},
		ProofType: m.proofType,
	}, nil
}

func (m *Mock) AcquireDeals(context.Context, abi.SectorID, api.AcquireDealsSpec) (api.Deals, error) {
	return nil, nil
}

func (m *Mock) AssignTicket(context.Context, abi.SectorID) (api.Ticket, error) {
	return m.ticket, nil
}

func (m *Mock) SubmitPreCommit(ctx context.Context, sector api.AllocatedSector, info api.PreCommitOnChainInfo) (api.SubmitPreCommitResp, error) {
	m.preCommits.Lock()
	defer m.preCommits.Unlock()

	if _, ok := m.preCommits.commits[sector.ID]; ok {
		return api.SubmitPreCommitResp{
			Res:  api.SubmitDuplicateSubmit,
			Desc: nil,
		}, nil
	}

	m.preCommits.commits[sector.ID] = info

	return api.SubmitPreCommitResp{
		Res:  api.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (m *Mock) PollPreCommitState(ctx context.Context, id abi.SectorID) (api.PollPreCommitStateResp, error) {
	m.preCommits.RLock()
	defer m.preCommits.RUnlock()

	if _, ok := m.preCommits.commits[id]; ok {
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

func (m *Mock) AssignSeed(context.Context, abi.SectorID) (api.Seed, error) {
	return m.seed, nil
}

func (m *Mock) SubmitProof(ctx context.Context, id abi.SectorID, info api.ProofOnChainInfo) (api.SubmitProofResp, error) {
	m.proofs.Lock()
	defer m.proofs.Unlock()

	if _, ok := m.proofs.proofs[id]; ok {
		return api.SubmitProofResp{
			Res:  api.SubmitDuplicateSubmit,
			Desc: nil,
		}, nil
	}

	m.proofs.proofs[id] = info

	return api.SubmitProofResp{
		Res:  api.SubmitAccepted,
		Desc: nil,
	}, nil
}

func (m *Mock) PollProofState(ctx context.Context, id abi.SectorID) (api.PollProofStateResp, error) {
	m.proofs.RLock()
	defer m.proofs.RUnlock()

	if _, ok := m.proofs.proofs[id]; ok {
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
