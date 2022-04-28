package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

var _ core.SealerAPI = (*Sealer)(nil)

func NewSealer(rand core.RandomnessAPI, sector core.SectorManager, deal core.DealManager, commit core.CommitmentManager) (*Sealer, error) {
	return &Sealer{
		rand:   rand,
		sector: sector,
		deal:   deal,
		commit: commit,
	}, nil
}

type Sealer struct {
	rand   core.RandomnessAPI
	sector core.SectorManager
	deal   core.DealManager
	commit core.CommitmentManager
}

func (s *Sealer) AllocateSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.AllocatedSector, error) {
	return s.sector.Allocate(ctx, spec)
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec) (core.Deals, error) {
	return s.deal.Acquire(ctx, sid, spec, core.SectorWorkerJobSealing)
}

func (s *Sealer) AssignTicket(ctx context.Context, sid abi.SectorID) (core.Ticket, error) {
	return s.rand.GetTicket(ctx, types.EmptyTSK, 0, sid.Miner)
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector core.AllocatedSector, info core.PreCommitOnChainInfo, reset bool) (core.SubmitPreCommitResp, error) {
	pinfo, err := info.IntoPreCommitInfo()
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}

	return s.commit.SubmitPreCommit(ctx, sector.ID, pinfo, reset)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (core.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) WaitSeed(ctx context.Context, sid abi.SectorID) (core.WaitSeedResp, error) {
	seed, err := s.rand.GetSeed(ctx, types.EmptyTSK, 0, sid.Miner)
	if err != nil {
		return core.WaitSeedResp{}, err
	}

	return core.WaitSeedResp{
		ShouldWait: false,
		Delay:      0,
		Seed:       &seed,
	}, nil
}

func (s *Sealer) SubmitPersisted(ctx context.Context, sid abi.SectorID, instance string) (bool, error) {
	log.Warnf("sector m-%d-s-%d is in the instance %s", sid.Miner, sid.Number, instance)
	return true, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info core.ProofInfo, reset bool) (core.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, reset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (core.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ListSectors(context.Context, core.SectorWorkerState) ([]*core.SectorState, error) {
	return nil, nil
}

func (s *Sealer) RestoreSector(context.Context, abi.SectorID, bool) (core.Meta, error) {
	return core.Empty, nil
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req core.ReportStateReq) (core.Meta, error) {
	log.Warnf("report state change for m-%d-s-%d: %#v", sid.Miner, sid.Number, req)
	return core.Empty, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (core.Meta, error) {
	log.Warnf("report finalized for m-%d-s-%d", sid.Miner, sid.Number)
	return core.Empty, nil
}

func (s *Sealer) ReportAborted(ctx context.Context, sid abi.SectorID, reason string) (core.Meta, error) {
	log.Warnf("report aborted for m-%d-s-%d: %s", sid.Miner, sid.Number, reason)
	return core.Empty, nil
}

func (s *Sealer) CheckProvable(ctx context.Context, mid abi.ActorID, sectors []builtin.ExtendedSectorInfo, strict bool) (map[abi.SectorNumber]string, error) {
	return nil, nil
}

func (s *Sealer) SimulateWdPoSt(context.Context, address.Address, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error {
	return nil
}

func (s *Sealer) AllocateSanpUpSector(ctx context.Context, spec core.AllocateSnapUpSpec) (*core.AllocatedSnapUpSector, error) {
	//TODO: impl
	return nil, nil
}

func (s *Sealer) SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo core.SnapUpOnChainInfo) (core.SubmitSnapUpProofResp, error) {
	//TODO: impl
	return core.SubmitSnapUpProofResp{Res: core.SubmitAccepted}, nil
}

func (s *Sealer) SnapUpPreFetch(ctx context.Context, mid abi.ActorID, dlindex *uint64) (*core.SnapUpFetchResult, error) {
	return &core.SnapUpFetchResult{}, nil
}

func (s *Sealer) SnapUpCandidates(ctx context.Context, mid abi.ActorID) ([]*bitfield.BitField, error) {
	return nil, nil
}

func (s *Sealer) ProvingSectorInfo(ctx context.Context, sid abi.SectorID) (core.ProvingSectorInfo, error) {
	return core.ProvingSectorInfo{}, nil
}

func (s *Sealer) WorkerPing(ctx context.Context, winfo core.WorkerInfo) (core.Meta, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "\t")
	err := enc.Encode(winfo)
	if err != nil {
		return core.Empty, fmt.Errorf("marshal worker info: %w", err)
	}

	log.Warnf("worker ping: \n%s", buf.String())
	return core.Empty, nil
}

func (s *Sealer) WorkerGetPingInfo(ctx context.Context, name string) (*core.WorkerPingInfo, error) {
	return nil, nil
}

func (s *Sealer) WorkerPingInfoList(ctx context.Context) ([]core.WorkerPingInfo, error) {
	return nil, nil
}

func (s *Sealer) SectorIndexerFind(ctx context.Context, indexType core.SectorIndexType, sid abi.SectorID) (core.SectorIndexLocation, error) {
	return core.SectorIndexLocation{
		Found:    false,
		Instance: "",
	}, nil
}
