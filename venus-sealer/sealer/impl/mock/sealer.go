package mock

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var _ api.SealerAPI = (*Sealer)(nil)

func NewSealer(rand api.RandomnessAPI, sector api.SectorManager, deal api.DealManager, commit api.CommitmentManager) (*Sealer, error) {
	return &Sealer{
		rand:   rand,
		sector: sector,
		deal:   deal,
		commit: commit,
	}, nil
}

type Sealer struct {
	rand   api.RandomnessAPI
	sector api.SectorManager
	deal   api.DealManager
	commit api.CommitmentManager
}

func (s *Sealer) AllocateSector(ctx context.Context, spec api.AllocateSectorSpec) (*api.AllocatedSector, error) {
	return s.sector.Allocate(ctx, spec.AllowedMiners, spec.AllowedProofTypes)
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec api.AcquireDealsSpec) (api.Deals, error) {
	return s.deal.Acquire(ctx, sid, spec.MaxDeals)
}

func (s *Sealer) AssignTicket(ctx context.Context, sid abi.SectorID) (api.Ticket, error) {
	return s.rand.GetTicket(ctx, types.EmptyTSK, 0, sid.Miner)
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector api.AllocatedSector, info api.PreCommitOnChainInfo, reset bool) (api.SubmitPreCommitResp, error) {
	pinfo, err := info.IntoPreCommitInfo()
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}

	return s.commit.SubmitPreCommit(ctx, sector.ID, pinfo, reset)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (api.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) WaitSeed(ctx context.Context, sid abi.SectorID) (api.WaitSeedResp, error) {
	seed, err := s.rand.GetSeed(ctx, types.EmptyTSK, 0, sid.Miner)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	return api.WaitSeedResp{
		ShouldWait: false,
		Delay:      0,
		Seed:       &seed,
	}, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofInfo, reset bool) (api.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, reset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req api.ReportStateReq) error {
	log.Warnf("report state change for m-%d-s-%d: %#v", sid.Miner, sid.Number, req)
	return nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) error {
	log.Warnf("report finalized for m-%d-s-%d", sid.Miner, sid.Number)
	return nil
}
