package sealer

import (
	"context"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	// "github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var (
	ErrSectorAllocated = fmt.Errorf("sector allocated")
)

var _ api.SealerAPI = (*Sealer)(nil)

var log = logging.New("sealing")

func sectorLogger(sid abi.SectorID) *logging.ZapLogger {
	return log.With("miner", sid.Miner, "num", sid.Number)
}

func New(
	capi chain.API,
	rand api.RandomnessAPI,
	sector api.SectorManager,
	state api.SectorStateManager,
	deal api.DealManager,
	commit api.CommitmentManager,
	sectorIdxer api.SectorIndexer,
	sectorTracker api.SectorTracker,
	prover api.Prover,
	// snapup api.SnapUpSectorManager,
) (*Sealer, error) {
	return &Sealer{
		capi:   capi,
		rand:   rand,
		sector: sector,
		state:  state,
		deal:   deal,
		commit: commit,
		// snapup: snapup,

		sectorIdxer:   sectorIdxer,
		sectorTracker: sectorTracker,

		prover: prover,
	}, nil
}

type Sealer struct {
	capi   chain.API
	rand   api.RandomnessAPI
	sector api.SectorManager
	state  api.SectorStateManager
	deal   api.DealManager
	commit api.CommitmentManager
	// snapup api.SnapUpSectorManager

	sectorIdxer   api.SectorIndexer
	sectorTracker api.SectorTracker

	prover api.Prover
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

func (s *Sealer) AcquireDeals(ctx context.Context, sid abi.SectorID, spec api.AcquireDealsSpec) (api.Deals, error) {
	state, err := s.state.Load(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("load sector state: %w", err)
	}

	if len(state.Pieces) != 0 {
		return state.Pieces, nil
	}

	pieces, err := s.deal.Acquire(ctx, sid, spec, api.SectorWorkerJobSealing)
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

	err = s.state.Update(ctx, sid, pieces)
	if err != nil {
		slog.Errorf("failed to update sector state: %v", err)
		return nil, err
	}

	success = true

	return pieces, nil
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
	state, err := s.state.Load(ctx, sid)
	if err != nil {
		return false, fmt.Errorf("load state for %s: %w", util.FormatSectorID(sid), err)
	}

	ok, err := s.checkPersistedFiles(ctx, sid, state.SectorType, instance, false)
	if err != nil {
		return false, fmt.Errorf("check persisted filed: %w", err)
	}

	if !ok {
		return false, nil
	}

	err = s.sectorIdxer.Normal().Update(ctx, sid, instance)
	if err != nil {
		return false, fmt.Errorf("unable to update sector indexer for sector id %d instance %s %w", sid, instance, err)
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
	// TODO: remove this guard

	seedEpoch := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
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

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req api.ReportStateReq) (api.Meta, error) {
	if err := s.state.Update(ctx, sid, &req); err != nil {
		return api.Empty, err
	}

	return api.Empty, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (api.Meta, error) {
	sectorLogger(sid).Debug("sector finalized")
	if err := s.state.Finalize(ctx, sid, func(st *api.SectorState) (bool, error) {
		// upgrading sectors are not finalized via api calls
		if st.Upgraded {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return api.Empty, err
	}

	return api.Empty, nil
}

func (s *Sealer) ReportAborted(ctx context.Context, sid abi.SectorID, reason string) (api.Meta, error) {
	err := s.state.Finalize(ctx, sid, func(st *api.SectorState) (bool, error) {
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
		return api.Empty, err
	}

	return api.Empty, nil
}

// snap
func (s *Sealer) AllocateSanpUpSector(ctx context.Context, spec api.AllocateSnapUpSpec) (*api.AllocatedSnapUpSector, error) {
	return nil, nil
}

func (s *Sealer) SubmitSnapUpProof(ctx context.Context, sid abi.SectorID, snapupInfo api.SnapUpOnChainInfo) (api.SubmitSnapUpProofResp, error) {
	desc := "not implemented yet"
	resp := api.SubmitSnapUpProofResp{}
	resp.Res = api.SubmitRejected
	resp.Desc = &desc
	return resp, nil
}

func (s *Sealer) checkPersistedFiles(ctx context.Context, sid abi.SectorID, proofType abi.RegisteredSealProof, instance string, upgrade bool) (bool, error) {
	locator := api.SectorLocator(func(lctx context.Context, lsid abi.SectorID) (string, bool, error) {
		if lsid != sid {
			return "", false, nil
		}

		return instance, true, nil
	})

	err := s.sectorTracker.SingleProvable(ctx, api.SectorRef{ID: sid, ProofType: proofType}, upgrade, locator, false)
	if err != nil {
		if errors.Is(err, objstore.ErrObjectNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("check provable for sector %s in instance %s: %w", util.FormatSectorID(sid), instance, err)
	}

	return true, nil
}

func checkPieces(pieces api.Deals) error {
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
