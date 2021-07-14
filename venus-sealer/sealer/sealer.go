package sealer

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var _ api.SealerAPI = (*Sealer)(nil)

var log = logging.New("sealing")

func New(capi chain.API, rand api.RandomnessAPI, sector api.SectorManager, deal api.DealManager, commit api.CommitmentManager) (*Sealer, error) {
	return &Sealer{
		rand:   rand,
		sector: sector,
		deal:   deal,
		commit: commit,
	}, nil
}

type Sealer struct {
	capi   chain.API
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
	panic("impl this")
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector api.AllocatedSector, info api.PreCommitOnChainInfo) (api.SubmitPreCommitResp, error) {
	return s.commit.SubmitPreCommit(ctx, sector.ID, info)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (api.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) AssignSeed(ctx context.Context, sid abi.SectorID) (api.Seed, error) {
	panic("impl this")
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofOnChainInfo) (api.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}
