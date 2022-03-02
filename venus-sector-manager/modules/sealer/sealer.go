package sealer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"

	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
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
	sectorfaultTracker api.SectorTracker,
	prover api.Prover,
) (*Sealer, error) {
	return &Sealer{
		capi:        capi,
		rand:        rand,
		sector:      sector,
		state:       state,
		deal:        deal,
		commit:      commit,
		sectorIdxer: sectorIdxer,

		sectorTracker: sectorfaultTracker,
		prover:        prover,
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

	sectorTracker api.SectorTracker
	prover        api.Prover
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
	state, err := s.state.Load(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("load sector state: %w", err)
	}

	if len(state.Deals) != 0 {
		return state.Deals, nil
	}

	deals, err := s.deal.Acquire(ctx, sid, spec.MaxDeals)
	if err != nil {
		return nil, err
	}

	success := false
	slog := sectorLogger(sid).With("total-pieces", len(deals))

	slog.Debugw("deals acquired")

	defer func() {
		if !success {
			if rerr := s.deal.Release(ctx, sid, deals); rerr != nil {
				slog.Errorf("failed to release deals %v", rerr)
			}
		}
	}()

	// validate deals
	for di := range deals {
		// should be a pledge piece
		dinfo := deals[di]
		if dinfo.ID == 0 {
			expected := zerocomm.ZeroPieceCommitment(dinfo.Piece.Size.Unpadded())
			if !expected.Equals(dinfo.Piece.Cid) {
				slog.Errorw("got unexpected non-deal piece", "piece-seq", di, "piece-size", dinfo.Piece.Size, "piece-cid", dinfo.Piece.Cid)
				return nil, fmt.Errorf("got unexpected non-deal piece")
			}
		}
	}

	err = s.state.Update(ctx, sid, deals)
	if err != nil {
		slog.Errorf("failed to update sector state: %v", err)
		return nil, err
	}

	success = true

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

func (s *Sealer) ListSectors(ctx context.Context, ws api.SectorWorkerState) ([]*api.SectorState, error) {
	return s.state.All(ctx, ws)
}

func (s *Sealer) ReportState(ctx context.Context, sid abi.SectorID, req api.ReportStateReq) (api.Meta, error) {
	if err := s.state.Update(ctx, sid, &req); err != nil {
		return api.Empty, err
	}

	return api.Empty, nil
}

func (s *Sealer) ReportFinalized(ctx context.Context, sid abi.SectorID) (api.Meta, error) {
	sectorLogger(sid).Debug("sector finalized")
	if err := s.state.Finalize(ctx, sid, nil); err != nil {
		return api.Empty, err
	}

	return api.Empty, nil
}

func (s *Sealer) ReportAborted(ctx context.Context, sid abi.SectorID, reason string) (api.Meta, error) {
	err := s.state.Finalize(ctx, sid, func(st *api.SectorState) error {
		if dealCount := len(st.Deals); dealCount > 0 {
			err := s.deal.Release(ctx, sid, st.Deals)
			if err != nil {
				return fmt.Errorf("release deals in sector: %w", err)
			}

			sectorLogger(sid).Debugw("deals released", "count", dealCount)
		}

		st.AbortReason = reason
		return nil
	})

	if err != nil {
		return api.Empty, err
	}

	return api.Empty, nil
}

func (s *Sealer) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storage.SectorRef, strict bool) (map[abi.SectorNumber]string, error) {

	return s.sectorTracker.Provable(ctx, pp, sectors, strict)
}

func (s *Sealer) SimulateWdPoSt(ctx context.Context, maddr address.Address, sis []builtin.ExtendedSectorInfo, rand abi.PoStRandomness) error {
	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return err
	}

	privSectors, err := s.sectorTracker.PubToPrivate(ctx, abi.ActorID(mid), sis)
	if err != nil {
		return fmt.Errorf("turn public sector infos into private: %w", err)
	}

	go func() {
		tCtx := context.TODO()

		tsStart := clock.NewSystemClock().Now()

		log.Info("mock generate window post start")
		_, _, err = s.prover.GenerateWindowPoSt(tCtx, abi.ActorID(mid), privSectors, append(abi.PoStRandomness{}, rand...))
		if err != nil {
			log.Warnf("generate window post failed: %v", err.Error())
			return
		}

		elapsed := time.Since(tsStart)
		log.Infow("mock generate window post", "elapsed", elapsed)
	}()

	return nil
}
