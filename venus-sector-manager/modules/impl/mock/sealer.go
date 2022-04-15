package mock

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
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
	return s.sector.Allocate(ctx, spec)
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec api.AcquireDealsSpec) (api.Deals, error) {
	return s.deal.Acquire(ctx, sid, spec, api.SectorWorkerJobSealing)
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

func (s *Sealer) SubmitPersisted(ctx context.Context, sid abi.SectorID, instance string) (bool, error) {
	log.Warnf("sector m-%d-s-%d is in the instance %s", sid.Miner, sid.Number, instance)
	return true, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info api.ProofInfo, reset bool) (api.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, reset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (api.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ListSectors(context.Context, api.SectorWorkerState) ([]*api.SectorState, error) {
	return nil, nil
}

func (s *Sealer) RestoreSector(context.Context, abi.SectorID, bool) (api.Meta, error) {
	return api.Empty, nil
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req api.ReportStateReq) (api.Meta, error) {
	log.Warnf("report state change for m-%d-s-%d: %#v", sid.Miner, sid.Number, req)
	return api.Empty, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (api.Meta, error) {
	log.Warnf("report finalized for m-%d-s-%d", sid.Miner, sid.Number)
	return api.Empty, nil
}

func (s *Sealer) ReportAborted(ctx context.Context, sid abi.SectorID, reason string) (api.Meta, error) {
	log.Warnf("report aborted for m-%d-s-%d: %s", sid.Miner, sid.Number, reason)
	return api.Empty, nil
}

func (s *Sealer) CheckProvable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
	return nil, nil
}

func (s *Sealer) SimulateWdPoSt(context.Context, address.Address, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error {
	return nil
}

func (s *Sealer) AllocateSanpUpSector(ctx context.Context, spec api.AllocateSnapUpSpec) (*api.AllocatedSnapUpSector, error) {
	//TODO: impl
	return nil, nil
}

func (s *Sealer) SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo api.SnapUpOnChainInfo) (api.SubmitSnapUpProofResp, error) {
	//TODO: impl
	return api.SubmitSnapUpProofResp{Res: api.SubmitAccepted}, nil
}

func (s *Sealer) SnapUpPreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*api.SnapUpFetchResult, error) {
	return &api.SnapUpFetchResult{}, nil
}

func (s *Sealer) SnapUpCandidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error) {
	return nil, nil
}

func (s *Sealer) ProvingSectorInfo(ctx context.Context, sid abi.SectorID) (api.ProvingSectorInfo, error) {
	return api.ProvingSectorInfo{}, nil
}
