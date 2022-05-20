package sealer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var (
	ErrSectorAllocated = fmt.Errorf("sector allocated")
)

var _ core.SealerAPI = (*Sealer)(nil)

var log = logging.New("sealing")

func sectorLogger(sid abi.SectorID) *logging.ZapLogger {
	return log.With("miner", sid.Miner, "num", sid.Number)
}

func sectorStateErr(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return fmt.Errorf("%w: sector state not found", core.APIErrCodeSectorStateNotFound)
	}

	return fmt.Errorf("sector state: %w", err)
}

func New(
	capi chain.API,
	rand core.RandomnessAPI,
	sector core.SectorManager,
	state core.SectorStateManager,
	deal core.DealManager,
	commit core.CommitmentManager,
	sectorIdxer core.SectorIndexer,
	sectorTracker core.SectorTracker,
	prover core.Prover,
	snapup core.SnapUpSectorManager,
	workerMgr core.WorkerManager,
) (*Sealer, error) {
	return &Sealer{
		capi:      capi,
		rand:      rand,
		sector:    sector,
		state:     state,
		deal:      deal,
		commit:    commit,
		snapup:    snapup,
		workerMgr: workerMgr,

		sectorIdxer:   sectorIdxer,
		sectorTracker: sectorTracker,

		prover: prover,
	}, nil
}

type Sealer struct {
	capi      chain.API
	rand      core.RandomnessAPI
	sector    core.SectorManager
	state     core.SectorStateManager
	deal      core.DealManager
	commit    core.CommitmentManager
	snapup    core.SnapUpSectorManager
	workerMgr core.WorkerManager

	sectorIdxer   core.SectorIndexer
	sectorTracker core.SectorTracker

	prover core.Prover
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

func (s *Sealer) AllocateSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.AllocatedSector, error) {
	sector, err := s.sector.Allocate(ctx, spec)
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

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec) (core.Deals, error) {
	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return nil, sectorStateErr(err)
	}

	if len(state.Pieces) != 0 {
		return state.Pieces, nil
	}

	pieces, err := s.deal.Acquire(ctx, sid, spec, core.SectorWorkerJobSealing)
	if err != nil {
		return nil, err
	}

	success := false
	slog := sectorLogger(sid).With("total-pieces", len(pieces))

	slog.Debugw("deals acquired")

	defer func() {
		if !success {
			if rerr := s.deal.Release(ctx, sid, pieces); rerr != nil {
				slog.Errorf("failed to release deals %v", rerr)
			}
		}
	}()

	// validate deals
	if err := checkPieces(pieces); err != nil {
		slog.Errorf("get invalid piece: %s", err)
		return nil, err
	}

	err = s.state.Update(ctx, sid, core.WorkerOnline, pieces)
	if err != nil {
		slog.Errorf("failed to update sector state: %v", err)
		return nil, sectorStateErr(err)
	}

	success = true

	return pieces, nil
}

func (s *Sealer) AssignTicket(ctx context.Context, sid abi.SectorID) (core.Ticket, error) {
	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return core.Ticket{}, err
	}

	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	ticket, err := s.rand.GetTicket(ctx, ts.Key(), ticketEpoch, sid.Miner)
	if err != nil {
		return core.Ticket{}, err
	}

	if err := s.state.Update(ctx, sid, core.WorkerOnline, &ticket); err != nil {
		return core.Ticket{}, sectorStateErr(err)
	}

	return ticket, nil
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector core.AllocatedSector, info core.PreCommitOnChainInfo, hardReset bool) (core.SubmitPreCommitResp, error) {
	pinfo, err := info.IntoPreCommitInfo()
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}
	return s.commit.SubmitPreCommit(ctx, sector.ID, pinfo, hardReset)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (core.PollPreCommitStateResp, error) {
	return s.commit.PreCommitState(ctx, sid)
}

func (s *Sealer) SubmitPersisted(ctx context.Context, sid abi.SectorID, instance string) (bool, error) {
	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return false, sectorStateErr(err)
	}

	ok, err := s.checkPersistedFiles(ctx, sid, state.SectorType, instance, false)
	if err != nil {
		return false, fmt.Errorf("check persisted filed: %w", err)
	}

	if !ok {
		return false, nil
	}

	err = s.sectorIdxer.Normal().Update(ctx, sid, core.SectorAccessStores{
		SealedFile: instance,
		CacheDir:   instance,
	})
	if err != nil {
		return false, fmt.Errorf("unable to update sector indexer for sector id %d instance %s %w", sid, instance, err)
	}

	return true, nil
}

func (s *Sealer) WaitSeed(ctx context.Context, sid abi.SectorID) (core.WaitSeedResp, error) {
	maddr, err := address.NewIDAddress(uint64(sid.Miner))
	if err != nil {
		return core.WaitSeedResp{}, err
	}

	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return core.WaitSeedResp{}, err
	}

	tsk := ts.Key()
	pci, err := s.capi.StateSectorPreCommitInfo(ctx, maddr, sid.Number, tsk)
	if err != nil {
		return core.WaitSeedResp{}, err
	}

	curEpoch := ts.Height()
	// TODO: remove this guard

	seedEpoch := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
	confEpoch := seedEpoch + policy.InteractivePoRepConfidence
	if curEpoch < confEpoch {
		return core.WaitSeedResp{
			ShouldWait: true,
			Delay:      int(confEpoch-curEpoch) * int(policy.NetParams.Network.BlockDelay),
			Seed:       nil,
		}, nil
	}

	seed, err := s.rand.GetSeed(ctx, tsk, seedEpoch, sid.Miner)
	if err != nil {
		return core.WaitSeedResp{}, err
	}

	if err := s.state.Update(ctx, sid, core.WorkerOnline, &seed); err != nil {
		return core.WaitSeedResp{}, sectorStateErr(err)
	}

	return core.WaitSeedResp{
		ShouldWait: false,
		Delay:      0,
		Seed:       &seed,
	}, nil
}

func (s *Sealer) SubmitProof(ctx context.Context, sid abi.SectorID, info core.ProofOnChainInfo, hardReset bool) (core.SubmitProofResp, error) {
	return s.commit.SubmitProof(ctx, sid, info, hardReset)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (core.PollProofStateResp, error) {
	return s.commit.ProofState(ctx, sid)
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req core.ReportStateReq) (core.Meta, error) {
	if err := s.state.Update(ctx, sid, core.WorkerOnline, &req); err != nil {
		return core.Empty, sectorStateErr(err)
	}

	return core.Empty, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (core.Meta, error) {
	sectorLogger(sid).Debug("sector finalized")
	if err := s.state.Finalize(ctx, sid, func(st *core.SectorState) (bool, error) {
		// upgrading sectors are not finalized via api calls
		if st.Upgraded {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return core.Empty, sectorStateErr(err)
	}

	return core.Empty, nil
}

func (s *Sealer) ReportAborted(ctx context.Context, sid abi.SectorID, reason string) (core.Meta, error) {
	err := s.state.Finalize(ctx, sid, func(st *core.SectorState) (bool, error) {
		if dealCount := len(st.Pieces); dealCount > 0 {
			err := s.deal.Release(ctx, sid, st.Pieces)
			if err != nil {
				return false, fmt.Errorf("release deals in sector: %w", err)
			}

			sectorLogger(sid).Debugw("deals released", "count", dealCount)
		}

		st.AbortReason = reason
		return true, nil
	})

	if err != nil {
		return core.Empty, sectorStateErr(err)
	}

	return core.Empty, nil
}

// snap
func (s *Sealer) AllocateSanpUpSector(ctx context.Context, spec core.AllocateSnapUpSpec) (*core.AllocatedSnapUpSector, error) {
	candidateSector, err := s.snapup.Allocate(ctx, spec.Sector)
	if err != nil {
		return nil, fmt.Errorf("allocate snapup sector: %w", err)
	}

	if candidateSector == nil {
		return nil, nil
	}

	alog := log.With("miner", candidateSector.Sector.ID.Miner, "sector", candidateSector.Sector.ID.Number)
	success := false

	defer func() {
		if success {
			return
		}

		alog.Debug("release allocated snapup sector")
		if rerr := s.snapup.Release(ctx, candidateSector); rerr != nil {
			alog.Errorf("release allocated snapup sector: %s", rerr)
		}
	}()

	pieces, err := s.deal.Acquire(ctx, candidateSector.Sector.ID, spec.Deals, core.SectorWorkerJobSnapUp)
	if err != nil {
		return nil, fmt.Errorf("acquire deals: %w", err)
	}

	if len(pieces) == 0 {
		return nil, nil
	}

	alog = alog.With("pieces", len(pieces))

	defer func() {
		if success {
			return
		}

		alog.Debug("release acquired deals")
		if rerr := s.deal.Release(ctx, candidateSector.Sector.ID, pieces); rerr != nil {
			alog.Errorf("release acquired deals: %s", err)
		}
	}()

	err = checkPieces(pieces)
	if err != nil {
		return nil, fmt.Errorf("check pieces: %w", err)
	}

	upgradePublic := core.SectorUpgradePublic(candidateSector.Public)

	err = s.state.Restore(ctx, candidateSector.Sector.ID, func(ss *core.SectorState) (bool, error) {
		// TODO more checks?
		ss.Pieces = pieces
		ss.Upgraded = true
		ss.UpgradePublic = &upgradePublic
		return true, nil
	})

	if err != nil && !errors.Is(err, kvstore.ErrKeyNotFound) {
		return nil, fmt.Errorf("restore sector for snapup: %w", err)
	}

	if err != nil {
		ierr := s.state.InitWith(ctx, candidateSector.Sector.ID, candidateSector.Sector.ProofType, core.SectorUpgraded(true), pieces, &upgradePublic)
		if ierr != nil {
			return nil, fmt.Errorf("init non-exist snapup sector: %w", err)
		}
	}

	success = true
	return &core.AllocatedSnapUpSector{
		Sector:  candidateSector.Sector,
		Pieces:  pieces,
		Public:  candidateSector.Public,
		Private: candidateSector.Private,
	}, nil
}

func (s *Sealer) SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo core.SnapUpOnChainInfo) (core.SubmitSnapUpProofResp, error) {
	resp := core.SubmitSnapUpProofResp{}
	desc := ""

	if len(snapupInfo.Proof) == 0 {
		desc = "empty proof is invalid"
		resp.Res = core.SubmitRejected
		resp.Desc = &desc
		return resp, nil
	}

	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return resp, sectorStateErr(err)
	}

	if len(state.Pieces) != len(snapupInfo.Pieces) {
		desc = fmt.Sprintf("pieces count not match: %d != %d", len(state.Pieces), len(snapupInfo.Pieces))
		resp.Res = core.SubmitRejected
		resp.Desc = &desc
		return resp, nil
	}

	for pi, pid := range snapupInfo.Pieces {
		if localPID := state.Pieces[pi].Piece.Cid; !pid.Equals(state.Pieces[pi].Piece.Cid) {
			desc = fmt.Sprintf("#%d piece cid not match: %s != %s", pi, localPID, pid)
			resp.Res = core.SubmitRejected
			resp.Desc = &desc
			return resp, nil
		}
	}

	newSealedCID, err := util.ReplicaCommitment2CID(snapupInfo.CommR)
	if err != nil {
		desc = fmt.Sprintf("convert comm_r to cid: %s", err)
		resp.Res = core.SubmitRejected
		resp.Desc = &desc
		return resp, nil
	}

	newUnsealedCID, err := util.DataCommitment2CID(snapupInfo.CommD)
	if err != nil {
		desc = fmt.Sprintf("convert comm_d to cid: %s", err)
		resp.Res = core.SubmitRejected
		resp.Desc = &desc
		return resp, nil
	}

	persisted, err := s.checkPersistedFiles(ctx, sid, state.SectorType, snapupInfo.AccessInstance, true)
	if err != nil {
		return resp, fmt.Errorf("check persisted files: %w", err)
	}

	if !persisted {
		desc = "not all files persisted"
		resp.Res = core.SubmitFilesMissed
		resp.Desc = &desc
		return resp, nil
	}

	upgradedInfo := core.SectorUpgradedInfo{
		SealedCID:      newSealedCID,
		UnsealedCID:    newUnsealedCID,
		Proof:          snapupInfo.Proof,
		AccessInstance: snapupInfo.AccessInstance,
	}

	err = s.state.Update(ctx, sid, core.WorkerOnline, &upgradedInfo)
	if err != nil {
		return resp, sectorStateErr(err)
	}

	err = s.snapup.Commit(ctx, sid)
	if err != nil {
		return resp, fmt.Errorf("commit snapup sector: %w", err)
	}

	// TODO: start submitting
	resp.Res = core.SubmitAccepted
	return resp, nil
}

func (s *Sealer) checkPersistedFiles(ctx context.Context, sid abi.SectorID, proofType abi.RegisteredSealProof, instance string, upgrade bool) (bool, error) {
	locator := core.SectorLocator(func(lctx context.Context, lsid abi.SectorID) (core.SectorAccessStores, bool, error) {
		if lsid != sid {
			return core.SectorAccessStores{}, false, nil
		}

		return core.SectorAccessStores{
			SealedFile: instance,
			CacheDir:   instance,
		}, true, nil
	})

	err := s.sectorTracker.SingleProvable(ctx, core.SectorRef{ID: sid, ProofType: proofType}, upgrade, locator, false)
	if err != nil {
		if errors.Is(err, objstore.ErrObjectNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("check provable for sector %s in instance %s: %w", util.FormatSectorID(sid), instance, err)
	}

	return true, nil
}

func checkPieces(pieces core.Deals) error {
	// validate deals
	for pi := range pieces {
		// should be a pledge piece
		pinfo := pieces[pi]
		if pinfo.ID == 0 {
			expected := zerocomm.ZeroPieceCommitment(pinfo.Piece.Size.Unpadded())
			if !expected.Equals(pinfo.Piece.Cid) {
				return fmt.Errorf("got unexpected non-deal piece with seq=#%d, size=%d, cid=%s", pi, pinfo.Piece.Size, pinfo.Piece.Cid)
			}
		}
	}

	return nil
}

func (s *Sealer) WorkerPing(ctx context.Context, winfo core.WorkerInfo) (core.Meta, error) {
	if winfo.Name == "" {
		return core.Empty, fmt.Errorf("worker name is required")
	}

	if winfo.Dest == "" {
		return core.Empty, fmt.Errorf("worker dest is required")
	}

	pingInfo := core.WorkerPingInfo{
		Info:     winfo,
		LastPing: time.Now().Unix(),
	}

	err := s.workerMgr.Update(ctx, pingInfo)
	if err != nil {
		return core.Empty, fmt.Errorf("update worker info: %w", err)
	}

	return core.Empty, nil
}
