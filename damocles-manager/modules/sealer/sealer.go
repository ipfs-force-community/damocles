package sealer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs/go-cid"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/metrics"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/piecestore"
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
	scfg *modules.SafeConfig,
	capi chain.API,
	rand core.RandomnessAPI,
	sector core.SectorManager,
	state core.SectorStateManager,
	deal core.DealManager,
	commit core.CommitmentManager,
	sectorIdxer core.SectorIndexer,
	sectorProving core.SectorProving,
	prover core.Prover,
	pieceStore piecestore.PieceStore,
	snapup core.SnapUpSectorManager,
	rebuild core.RebuildSectorManager,
	unseal core.UnsealSectorManager,
	workerMgr core.WorkerManager,
) (*Sealer, error) {
	return &Sealer{
		scfg:       scfg,
		capi:       capi,
		rand:       rand,
		sector:     sector,
		state:      state,
		deal:       deal,
		commit:     commit,
		snapup:     snapup,
		rebuild:    rebuild,
		unseal:     unseal,
		workerMgr:  workerMgr,
		pieceStore: pieceStore,

		sectorIdxer:   sectorIdxer,
		sectorProving: sectorProving,

		prover: prover,
	}, nil
}

type Sealer struct {
	scfg       *modules.SafeConfig
	capi       chain.API
	rand       core.RandomnessAPI
	sector     core.SectorManager
	state      core.SectorStateManager
	deal       core.DealManager
	commit     core.CommitmentManager
	snapup     core.SnapUpSectorManager
	rebuild    core.RebuildSectorManager
	unseal     core.UnsealSectorManager
	workerMgr  core.WorkerManager
	pieceStore piecestore.PieceStore

	sectorIdxer   core.SectorIndexer
	sectorProving core.SectorProving

	prover core.Prover
}

// checkSectorNumbers returns the allocated sector numbers in `sids`
func (s *Sealer) checkSectorNumbers(ctx context.Context, mid abi.ActorID, sids []uint64) ([]uint64, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return []uint64{}, err
	}

	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return []uint64{}, err
	}

	allocated, err := s.capi.StateMinerAllocated(ctx, maddr, ts.Key())
	if err != nil {
		return []uint64{}, err
	}

	toBeAllocated := bitfield.NewFromSet(sids)
	duplicates, err := bitfield.IntersectBitField(*allocated, toBeAllocated)
	if err != nil {
		return []uint64{}, fmt.Errorf("calculate duplicate allocated sectors: %w", err)
	}
	return duplicates.All(uint64(len(sids)))
}

func (s *Sealer) AllocateSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.AllocatedSector, error) {
	sectors, err := s.AllocateSectorsBatch(ctx, spec, 1)
	if err != nil {
		return nil, err
	}
	if len(sectors) > 0 {
		return sectors[0], nil
	}
	return nil, nil
}

func (s *Sealer) AllocateSectorsBatch(ctx context.Context, spec core.AllocateSectorSpec, count uint32) ([]*core.AllocatedSector, error) {
	sectors, err := s.sector.Allocate(ctx, spec, count)
	if err != nil {
		return nil, err
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	sectorsIDs := make([]uint64, len(sectors))
	for i, sector := range sectors {
		sectorsIDs[i] = uint64(sector.ID.Number)
	}
	duplicates, err := s.checkSectorNumbers(ctx, sectors[0].ID.Miner, sectorsIDs)
	if err != nil {
		return nil, err
	}

	if len(duplicates) > 0 {
		ss := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(duplicates)), ","), "[]")
		return nil, fmt.Errorf("%w. miner: %d, sectors: (%s)", ErrSectorAllocated, sectors[0].ID.Miner, ss)
	}

	if err := s.state.Init(ctx, sectors, core.WorkerOnline); err != nil {
		return nil, err
	}
	ctx, _ = metrics.New(ctx, metrics.Upsert(metrics.Miner, sectors[0].ID.Miner.String()))
	metrics.Record(ctx, metrics.SectorManagerNewSector.M(int64(len(sectors))))

	return sectors, nil
}

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec core.AcquireDealsSpec) (core.Deals, error) {
	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return nil, sectorStateErr(err)
	}

	if len(state.Pieces) != 0 {
		return state.Pieces, nil
	}
	s.scfg.Lock()
	mcfg, err := s.scfg.MinerConfig(sid.Miner)
	s.scfg.Unlock()
	if err != nil {
		return nil, err
	}

	pieces := core.Deals{}
	if mcfg.Sealing.SealingEpochDuration != 0 {
		var h *types.TipSet
		h, err = s.capi.ChainHead(ctx)
		if err != nil {
			return nil, fmt.Errorf("get chain head: %w", err)
		}

		pieces, err = s.deal.Acquire(ctx, sid, spec, &core.AcquireDealsLifetime{
			Start: h.Height() + abi.ChainEpoch(mcfg.Sealing.SealingEpochDuration),
			End:   1<<63 - 1,
		}, core.SectorWorkerJobSealing)
	} else {
		pieces, err = s.deal.Acquire(ctx, sid, spec, nil, core.SectorWorkerJobSealing)
	}
	if err != nil {
		return nil, err
	}

	success := false
	slog := sectorLogger(sid).With("total-pieces", len(pieces))

	slog.Infow("deals acquired")

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

func (s *Sealer) AssignTicketBatch(ctx context.Context, minerID abi.ActorID, sectorIDs []abi.SectorNumber) (core.Ticket, error) {
	ts, err := s.capi.ChainHead(ctx)
	if err != nil {
		return core.Ticket{}, err
	}
	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	ticket, err := s.rand.GetTicket(ctx, ts.Key(), ticketEpoch, minerID)
	if err != nil {
		return core.Ticket{}, err
	}

	for _, sid := range sectorIDs {
		if err := s.state.Update(ctx, abi.SectorID{
			Miner:  minerID,
			Number: sid,
		}, core.WorkerOnline, &ticket); err != nil {
			return core.Ticket{}, sectorStateErr(err)
		}
	}
	return ticket, nil
}

func (s *Sealer) SubmitPreCommit(ctx context.Context, sector core.AllocatedSector, info core.PreCommitOnChainInfo, hardReset bool) (core.SubmitPreCommitResp, error) {
	pinfo, err := info.IntoPreCommitInfo()
	if err != nil {
		return core.SubmitPreCommitResp{}, err
	}

	resp, err := s.commit.SubmitPreCommit(ctx, sector.ID, pinfo, hardReset)
	if err == nil {
		ctx, _ = metrics.New(ctx, metrics.Upsert(metrics.Miner, sector.ID.Miner.String()))
		metrics.Record(ctx, metrics.SectorManagerPreCommitSector.M(1))
	}
	return resp, sectorStateErr(err)
}

func (s *Sealer) PollPreCommitState(ctx context.Context, sid abi.SectorID) (core.PollPreCommitStateResp, error) {
	resp, err := s.commit.PreCommitState(ctx, sid)
	return resp, sectorStateErr(err)
}

func (s *Sealer) SubmitPersisted(ctx context.Context, sid abi.SectorID, instance string) (bool, error) {
	return s.SubmitPersistedEx(ctx, sid, instance, false)
}

func (s *Sealer) SubmitPersistedEx(ctx context.Context, sid abi.SectorID, instance string, isUpgrade bool) (bool, error) {
	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return false, sectorStateErr(err)
	}

	ok, err := s.checkPersistedFiles(ctx, sid, state.SectorType, instance, isUpgrade)
	if err != nil {
		return false, fmt.Errorf("check persisted filed: %w", err)
	}

	if !ok {
		return false, nil
	}

	var indexer core.SectorTypedIndexer
	if isUpgrade {
		indexer = s.sectorIdxer.Upgrade()
	} else {
		indexer = s.sectorIdxer.Normal()
	}

	err = indexer.Update(ctx, sid, core.SectorAccessStores{
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
	if pci == nil {
		return core.WaitSeedResp{}, fmt.Errorf("precommit info not found on chain. sid: %s", util.FormatSectorID(sid))
	}

	curEpoch := ts.Height()
	// TODO: remove this guard

	seedEpoch := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
	confEpoch := seedEpoch + policy.InteractivePoRepConfidence
	if curEpoch < confEpoch {
		return core.WaitSeedResp{
			ShouldWait: true,
			Delay:      int(confEpoch-curEpoch) * int(policy.NetParams.BlockDelaySecs),
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
	resp, err := s.commit.SubmitProof(ctx, sid, info, hardReset)
	if err == nil {
		ctx, _ = metrics.New(ctx, metrics.Upsert(metrics.Miner, sid.Miner.String()))
		metrics.Record(ctx, metrics.SectorManagerCommitSector.M(1))
	}
	return resp, sectorStateErr(err)
}

func (s *Sealer) PollProofState(ctx context.Context, sid abi.SectorID) (core.PollProofStateResp, error) {
	resp, err := s.commit.ProofState(ctx, sid)
	return resp, sectorStateErr(err)
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req core.ReportStateReq) (*core.SectorStateResp, error) {
	state, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, sectorStateErr(err)
		}

		state, err = s.state.Load(ctx, sid, core.WorkerOffline)
		if nil != err {
			return nil, sectorStateErr(err)
		}
	} else {
		if err := s.state.Update(ctx, sid, core.WorkerOnline, &req); err != nil {
			return nil, sectorStateErr(err)
		}
	}

	return &core.SectorStateResp{
		ID:          state.ID,
		Finalized:   state.Finalized,
		AbortReason: &state.AbortReason,
	}, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (core.Meta, error) {
	sectorLogger(sid).Info("sector finalized")
	if err := s.state.Finalize(ctx, sid, func(st *core.SectorState) (bool, error) {

		// Upgrading sectors are not finalized via api calls
		// Except in the case of sector rebuild and unseal, because the prerequisite for sector rebuild is that the sector has been finalized.
		if bool(st.Unsealing) {
			st.Unsealing = false
		} else if bool(st.NeedRebuild) {
			st.NeedRebuild = false
		} else if bool(st.Upgraded) {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return core.Empty, sectorStateErr(err)
	}

	if _, err := s.sectorIdxer.StoreMgr().ReleaseReserved(ctx, sid); err != nil {
		log.With("sector", util.FormatSectorID(sid)).Errorf("release reserved: %s", err)
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

			sectorLogger(sid).Infow("deals released", "count", dealCount)
		}

		if st.Upgraded {
			s.snapup.CancelCommitment(ctx, sid)
		}

		st.AbortReason = reason
		return true, nil
	})

	if err != nil {
		return core.Empty, sectorStateErr(err)
	}

	if _, err := s.sectorIdxer.StoreMgr().ReleaseReserved(ctx, sid); err != nil {
		log.With("sector", util.FormatSectorID(sid)).Errorf("release reserved: %s", err)
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

	pieces, err := s.deal.Acquire(ctx, candidateSector.Sector.ID, spec.Deals, &core.AcquireDealsLifetime{
		Start:            candidateSector.Public.Activation,
		End:              candidateSector.Public.Expiration,
		SectorExpiration: &candidateSector.Public.Expiration,
	}, core.SectorWorkerJobSnapUp)
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

	if !checkForLifetime(candidateSector.Public, pieces) {
		alog.Warn("lifetimes of deals and sector do not match")
		return nil, nil
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
		ierr := s.state.InitWith(ctx, []*core.AllocatedSector{{ID: candidateSector.Sector.ID, ProofType: candidateSector.Sector.ProofType}}, core.WorkerOnline, core.SectorUpgraded(true), pieces, &upgradePublic)
		if ierr != nil {
			return nil, fmt.Errorf("init non-exist snapup sector: %w", ierr)
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
	nv, err := s.capi.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return false, fmt.Errorf("get network version: %w", err)
	}
	ppt, err := proofType.RegisteredWindowPoStProofByNetworkVersion(nv)
	if err != nil {
		return false, fmt.Errorf("convert to v1_1 post proof: %w", err)
	}
	err = s.sectorProving.SingleProvable(ctx, ppt, core.SectorRef{ID: sid, ProofType: proofType}, upgrade, locator, false, false)
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
		} else {
			if pinfo.Proposal == nil {
				return fmt.Errorf("get nil proposal for non-zero deal id: %d", pinfo.ID)
			}
		}
	}

	return nil
}

func checkForLifetime(public core.SectorPublicInfo, pieces core.Deals) bool {
	// validate deals
	for pi := range pieces {
		// should be a pledge piece
		pinfo := pieces[pi]
		if pinfo.ID == 0 {
			continue
		}

		if public.Activation > pinfo.Proposal.StartEpoch {
			return false
		}

		if pinfo.Proposal.EndEpoch >= public.Expiration {
			return false
		}
	}

	return true
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

func (s *Sealer) StoreReserveSpace(ctx context.Context, sid abi.SectorID, size uint64, candidates []string) (*core.StoreBasicInfo, error) {
	// TODO: check state?
	_, err := s.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return nil, sectorStateErr(err)
	}

	storeCfg, err := s.sectorIdxer.StoreMgr().ReserveSpace(ctx, sid, size, candidates)
	if err != nil {
		return nil, fmt.Errorf("reserve space: %w", err)
	}

	if storeCfg == nil {
		return nil, nil
	}

	basic := storeConfig2StoreBasic(storeCfg)
	return &basic, nil
}

func (s *Sealer) StoreBasicInfo(ctx context.Context, instanceName string) (*core.StoreBasicInfo, error) {
	store, err := s.sectorIdxer.StoreMgr().GetInstance(ctx, instanceName)
	if err != nil {
		if errors.Is(err, objstore.ErrObjectStoreInstanceNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("get store %s: %w", instanceName, err)
	}

	storeCfg := store.InstanceConfig(ctx)
	basic := storeConfig2StoreBasic(&storeCfg)
	return &basic, nil
}

func (s *Sealer) AllocateRebuildSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorRebuildInfo, error) {
	info, err := s.rebuild.Allocate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("allocate rebuild sector: %w", err)
	}

	if info == nil {
		return nil, nil
	}

	_, err = s.state.Load(ctx, info.Sector.ID, core.WorkerOnline)
	if err != nil {
		return nil, fmt.Errorf("load sector state from online database: %w", err)
	}

	return info, nil
}

func (s *Sealer) AllocateUnsealSector(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorUnsealInfo, error) {
	info, err := s.unseal.Allocate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("allocate unseal sector: %w", err)
	}

	if info == nil {
		return nil, nil
	}

	// get precommit info
	sectorState, err := s.state.Load(ctx, info.Sector.ID, core.WorkerOffline)
	if err != nil {
		return nil, fmt.Errorf("load sector state: %w", err)
	}

	preOnChain, err := sectorState.Pre.IntoPreCommitOnChainInfo()
	if err != nil {
		return nil, fmt.Errorf("load sector state: %w", err)
	}

	info.CommD = preOnChain.CommD
	info.Ticket = preOnChain.Ticket

	// get private info
	access, found, err := s.sectorIdxer.Normal().Find(ctx, info.Sector.ID)
	if err != nil {
		return nil, fmt.Errorf("find sector(%s) access store: %w", util.FormatSectorID(info.Sector.ID), err)
	}
	if !found {
		return nil, fmt.Errorf("sector(%s) access store not found", util.FormatSectorID(info.Sector.ID))
	}
	info.PrivateInfo.AccessInstance = access.SealedFile

	err = s.state.Restore(ctx, info.Sector.ID, func(st *core.SectorState) (bool, error) {
		st.Unsealing = true
		return true, nil
	})

	if err != nil && !errors.Is(err, kvstore.ErrKeyNotFound) {
		return nil, fmt.Errorf("restore sector for unseal: %w", err)
	}

	return info, nil
}

func (s *Sealer) AchieveUnsealSector(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, errInfo string) (core.Meta, error) {
	return core.Empty, s.unseal.Achieve(ctx, sid, pieceCid, errInfo)
}

func (s *Sealer) AcquireUnsealDest(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid) ([]string, error) {
	return s.unseal.AcquireDest(ctx, sid, pieceCid)
}
