package sealer

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/policy"
)

var (
	ErrSectorAllocated = fmt.Errorf("sector allocated")
)

var _ api.SealerAPI = (*Sealer)(nil)

var log = logging.New("sealing")

func New(capi chain.API, rand api.RandomnessAPI, sector api.SectorManager, state api.SectorStateManager, deal api.DealManager, commit api.CommitmentManager) (*Sealer, error) {
	return &Sealer{
		rand:   rand,
		sector: sector,
		state:  state,
		deal:   deal,
		commit: commit,
	}, nil
}

type Sealer struct {
	capi   chain.API
	rand   api.RandomnessAPI
	sector api.SectorManager
	state  api.SectorStateManager
	deal   api.DealManager
	commit api.CommitmentManager
}

func (s *Sealer) checkSectorNumber(ctx context.Context, sid abi.SectorID) (bool, error) {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return false, err
	}

	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return false, err
	}

	allocated, err := s.capi.StateMinerSectorAllocated(ctx, maddr, sid.Number, ts.Key())
	if err != nil {
		return false, err
	}

	return allocated, err
}

func (s *Sealer) AllocateSector(ctx context.Context, spec api.AllocateSectorSpec) (*api.AllocatedSector, error) {
	sector, err := s.sector.Allocate(ctx, spec.AllowedMiners, spec.AllowedProofTypes)
	if err != nil {
		return nil, err
	}

	if sector == nil {
		return nil, nil
	}

	allocated, err := s.checkSectorNumber(ctx, sector.ID)
	if err != nil {
		return nil, err
	}

	if allocated {
		return nil, fmt.Errorf("%w: m-%d-s-%d", ErrSectorAllocated, sector.ID.Miner, sector.ID.Number)
	}

	if err := s.state.Init(ctx, sector.ID); err != nil {
		return nil, err
	}

	return sector, nil
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec api.AcquireDealsSpec) (api.Deals, error) {
	return s.deal.Acquire(ctx, sid, spec.MaxDeals)
}

func (s *Sealer) getRandomnessEntropy(mid abi.ActorID) ([]byte, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := maddr.MarshalCBOR(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *Sealer) AssignTicket(ctx context.Context, sid abi.SectorID) (api.Ticket, error) {
	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return api.Ticket{}, err
	}

	entropy, err := s.getRandomnessEntropy(sid.Miner)
	if err != nil {
		return api.Ticket{}, nil
	}

	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	return s.rand.GetTicket(ctx, ts.Key(), ticketEpoch, entropy)
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector api.AllocatedSector, info api.PreCommitOnChainInfo) (api.SubmitPreCommitResp, error) {
	return s.commit.SubmitPreCommit(ctx, sector.ID, info)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (api.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) WaitSeed(ctx context.Context, sid abi.SectorID) (api.WaitSeedResp, error) {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	tsk := ts.Key()
	pci, err := s.capi.StateSectorPreCommitInfo(ctx, maddr, sid.Number, tsk)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	curEpoch := ts.Height()
	seedEpoch := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
	confEpoch := seedEpoch + policy.InteractivePoRepConfidence
	if curEpoch < confEpoch {
		return api.WaitSeedResp{
			ShouldWait: true,
			Delay:      int((confEpoch - curEpoch) * policy.EpochDurationSeconds),
			Seed:       nil,
		}, nil
	}

	entropy, err := s.getRandomnessEntropy(sid.Miner)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	seed, err := s.rand.GetSeed(ctx, tsk, seedEpoch, entropy)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	return api.WaitSeedResp{
		ShouldWait: false,
		Delay:      0,
		Seed:       &seed,
	}, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofOnChainInfo) (api.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}
