package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	chainAPI "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/ver"
)

var _ core.SealerAPI = (*Sealer)(nil)

func NewSealer(rand core.RandomnessAPI, sector core.SectorManager, deal core.DealManager, commit core.CommitmentManager,
	api chainAPI.API, scfg modules.SafeConfig,
) (*Sealer, error) {
	return &Sealer{
		rand:   rand,
		sector: sector,
		deal:   deal,
		commit: commit,
		api:    api,
		scfg:   scfg,
	}, nil
}

type Sealer struct {
	rand   core.RandomnessAPI
	sector core.SectorManager
	deal   core.DealManager
	commit core.CommitmentManager
	api    chainAPI.API
	scfg   modules.SafeConfig
}

func (s *Sealer) AllocateSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.AllocatedSector, error) {
	sectors, err := s.AllocateSectorsBatch(ctx, spec, 1)
	if err != nil {
		return nil, err
	}
	return sectors[0], nil
}

func (s *Sealer) AllocateSectorsBatch(ctx context.Context, spec core.AllocateSectorSpec, count uint32) ([]*core.AllocatedSector, error) {
	return s.sector.Allocate(ctx, spec, count)
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec) (core.Deals, error) {
	s.scfg.Lock()
	mcfg, err := s.scfg.MinerConfig(sid.Miner)
	s.scfg.Unlock()
	if err != nil {
		return nil, err
	}

	if mcfg.Sealing.SealingEpochDuration != 0 {
		h, err := s.api.ChainHead(ctx)
		if err != nil {
			return nil, fmt.Errorf("get chain head: %w", err)
		}
		lifetime := &core.AcquireDealsLifetime{}
		lifetime.Start = h.Height() + abi.ChainEpoch(mcfg.Sealing.SealingEpochDuration)
		lifetime.End = 1<<63 - 1

		return s.deal.Acquire(ctx, sid, spec, lifetime, core.SectorWorkerJobSealing)
	}
	return s.deal.Acquire(ctx, sid, spec, nil, core.SectorWorkerJobSealing)
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
	return s.SubmitPersistedEx(ctx, sid, instance, false)
}

func (s *Sealer) SubmitPersistedEx(ctx context.Context, sid abi.SectorID, instanceName string, isUpgrade bool) (bool, error) {
	log.Warnf("sector m-%d-s-%d(up=%v) is in the instance %s", sid.Miner, sid.Number, isUpgrade, instanceName)
	return true, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info core.ProofInfo, reset bool) (core.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, reset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (core.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ListSectors(context.Context, core.SectorWorkerState, core.SectorWorkerJob) ([]*core.SectorState, error) {
	return nil, nil
}

func (s *Sealer) FindSector(ctx context.Context, state core.SectorWorkerState, sid abi.SectorID) (*core.SectorState, error) {
	return nil, nil
}

func (s *Sealer) FindSectorsWithDeal(ctx context.Context, state core.SectorWorkerState, dealID abi.DealID) ([]*core.SectorState, error) {
	return nil, nil
}

func (s *Sealer) FindSectorWithPiece(ctx context.Context, state core.SectorWorkerState, pieceCid cid.Cid) (*core.SectorState, error) {
	return nil, nil
}

func (s *Sealer) ImportSector(ctx context.Context, ws core.SectorWorkerState, state *core.SectorState, override bool) (bool, error) {
	return false, nil
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

func (s *Sealer) CheckProvable(ctx context.Context, mid abi.ActorID, postProofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, strict, sateCheck bool) (map[abi.SectorNumber]string, error) {
	return nil, nil
}

func (s *Sealer) SimulateWdPoSt(context.Context, address.Address, abi.RegisteredPoStProof, []builtin.ExtendedSectorInfo, abi.PoStRandomness) error {
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

func (s *Sealer) SnapUpCancelCommitment(ctx context.Context, sid abi.SectorID) error {
	return nil
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

func (s *Sealer) WorkerPingInfoRemove(ctx context.Context, name string) error {
	return nil
}

func (s *Sealer) SectorIndexerFind(ctx context.Context, indexType core.SectorIndexType, sid abi.SectorID) (core.SectorIndexLocation, error) {
	return core.SectorIndexLocation{
		Found:    false,
		Instance: core.SectorAccessStores{},
	}, nil
}

func (s *Sealer) TerminateSector(ctx context.Context, sid abi.SectorID) (core.SubmitTerminateResp, error) {
	return s.commit.SubmitTerminate(ctx, sid)
}

func (s *Sealer) PollTerminateSectorState(ctx context.Context, sid abi.SectorID) (core.TerminateInfo, error) {
	return s.commit.TerminateState(ctx, sid)
}

func (s *Sealer) RemoveSector(ctx context.Context, sid abi.SectorID) error {
	return nil
}

func (s *Sealer) FinalizeSector(context.Context, abi.SectorID) error {
	return nil
}

func (s *Sealer) StoreReserveSpace(ctx context.Context, sid abi.SectorID, size uint64, candidates []string) (*core.StoreBasicInfo, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	selected := rand.Intn(len(candidates))
	selectedName := candidates[selected]

	log.Warnw("store for reserved space", "selected", selectedName)

	return &core.StoreBasicInfo{
		Name: selectedName,
		Path: selectedName,
		Meta: map[string]string{},
	}, nil
}

func (s *Sealer) StoreReleaseReserved(ctx context.Context, sid abi.SectorID) (bool, error) {
	return true, nil
}

func (s *Sealer) StoreList(ctx context.Context) ([]core.StoreDetailedInfo, error) {
	return nil, nil
}

func (s *Sealer) StoreBasicInfo(ctx context.Context, instanceName string) (*core.StoreBasicInfo, error) {
	log.Warnw("get store basic info", "instance", instanceName)
	return &core.StoreBasicInfo{
		Name: instanceName,
		Path: instanceName,
		Meta: map[string]string{},
	}, nil
}

func (s *Sealer) SectorSetForRebuild(ctx context.Context, sid abi.SectorID, opt core.RebuildOptions) (bool, error) {
	return false, nil
}

func (s *Sealer) AllocateRebuildSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorRebuildInfo, error) {
	return nil, nil
}

func (s *Sealer) UnsealPiece(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, offset types.UnpaddedByteIndex, size abi.UnpaddedPieceSize, dest string) (<-chan []byte, error) {
	return nil, nil
}

func (s *Sealer) AllocateUnsealSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorUnsealInfo, error) {
	return nil, nil
}

func (s *Sealer) AchieveUnsealSector(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, errInfo string) (core.Meta, error) {
	return nil, nil
}

func (s *Sealer) AcquireUnsealDest(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid) ([]string, error) {
	return nil, nil
}

func (s *Sealer) Version(context.Context) (string, error) {
	return ver.VersionStr(), nil
}
