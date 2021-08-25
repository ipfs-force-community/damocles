package sealer

import (
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/objstore"
	"github.com/dtynn/venus-cluster/venus-sector-manager/api"
	"github.com/dtynn/venus-cluster/venus-sector-manager/sealer/util"

	"github.com/dtynn/venus-cluster/venus-sector-manager/sealer/policy"
)

var (
	ErrSectorAllocated = fmt.Errorf("sector allocated")
)

var _ api.SealerAPI = (*Sealer)(nil)

var log = logging.New("sealing")

func sectorLogger(sid abi.SectorID) *logging.ZapLogger {
	return log.With("miner", sid.Miner, "num", sid.Number)
}

func New(capi chain.API, rand api.RandomnessAPI, sector api.SectorManager, state api.SectorStateManager, deal api.DealManager, commit api.CommitmentManager, sectorIdxer api.SectorIndexer) (*Sealer, error) {
	return &Sealer{
		capi:        capi,
		rand:        rand,
		sector:      sector,
		state:       state,
		deal:        deal,
		commit:      commit,
		sectorIdxer: sectorIdxer,
	}, nil
}

type Sealer struct {
	capi        chain.API
	rand        api.RandomnessAPI
	sector      api.SectorManager
	state       api.SectorStateManager
	deal        api.DealManager
	commit      api.CommitmentManager
	sectorIdxer api.SectorIndexer
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

	if err := s.state.Init(ctx, sector.ID, sector.ProofType); err != nil {
		return nil, err
	}

	return sector, nil
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec api.AcquireDealsSpec) (api.Deals, error) {
	deals, err := s.deal.Acquire(ctx, sid, spec.MaxDeals)
	if err != nil {
		return nil, err
	}

	err = s.state.Update(ctx, sid, deals)
	if err != nil {
		if rerr := s.deal.Release(ctx, deals); rerr != nil {
			sectorLogger(sid).Errorf("failed to release deals %v", deals)
		}
		return nil, err
	}

	return deals, nil
}

func (s *Sealer) AssignTicket(ctx context.Context, sid abi.SectorID) (api.Ticket, error) {
	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return api.Ticket{}, err
	}

	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	ticket, err := s.rand.GetTicket(ctx, ts.Key(), ticketEpoch, sid.Miner)
	if err != nil {
		return api.Ticket{}, err
	}

	if err := s.state.Update(ctx, sid, &ticket); err != nil {
		return api.Ticket{}, err
	}

	return ticket, nil
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector api.AllocatedSector, info api.PreCommitOnChainInfo, hardReset bool) (api.SubmitPreCommitResp, error) {
	pinfo, err := info.IntoPreCommitInfo()
	if err != nil {
		return api.SubmitPreCommitResp{}, err
	}
	return s.commit.SubmitPreCommit(ctx, sector.ID, pinfo, hardReset)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (api.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) SubmitPersisted(ctx context.Context, sid abi.SectorID, instance string) (bool, error) {
	ins, err := s.sectorIdxer.StoreMgr().GetInstance(ctx, instance)
	if err != nil {
		if errors.Is(err, objstore.ErrObjectStoreInstanceNotFound) {
			return false, nil
		}

		return false, err
	}

	// check for sealed file existance
	reader, err := ins.Get(ctx, util.SectorPath(util.SectorPathTypeSealed, sid))
	if err != nil {
		if errors.Is(err, objstore.ErrObjectNotFound) {
			return false, nil
		}

		return false, err
	}

	reader.Close()

	err = s.sectorIdxer.Update(ctx, sid, instance)
	if err != nil {
		return false, err
	}

	return true, nil
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
	seedEpoch := pci.PreCommitEpoch + policy.NetParams.Network.PreCommitChallengeDelay
	confEpoch := seedEpoch + policy.InteractivePoRepConfidence
	if curEpoch < confEpoch {
		return api.WaitSeedResp{
			ShouldWait: true,
			Delay:      int(confEpoch-curEpoch) * int(policy.NetParams.Network.BlockDelay),
			Seed:       nil,
		}, nil
	}

	seed, err := s.rand.GetSeed(ctx, tsk, seedEpoch, sid.Miner)
	if err != nil {
		return api.WaitSeedResp{}, err
	}

	if err := s.state.Update(ctx, sid, &seed); err != nil {
		return api.WaitSeedResp{}, err
	}

	return api.WaitSeedResp{
		ShouldWait: false,
		Delay:      0,
		Seed:       &seed,
	}, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofOnChainInfo, hardReset bool) (api.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, hardReset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req api.ReportStateReq) error {
	if err := s.state.Update(ctx, sid, &req); err != nil {
		return err
	}

	return nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) error {
	if err := s.state.Finalize(ctx, sid); err != nil {
		return err
	}

	return nil
}
