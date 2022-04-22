package sectors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

func NewSnapUpCommitter(
	ctx context.Context,
	tracker core.SectorTracker,
	indexer core.SectorIndexer,
	chainAPI chain.API,
	eventbus *chain.EventBus,
	messagerAPI messager.API,
	stateMgr core.SectorStateManager,
	scfg *modules.SafeConfig,
) (*SnapUpCommitter, error) {
	ctx, cancel := context.WithCancel(ctx)
	committer := &SnapUpCommitter{
		ctx:      ctx,
		cancel:   cancel,
		tracker:  tracker,
		indexer:  indexer,
		chain:    chainAPI,
		eventbus: eventbus,
		messager: messagerAPI,
		state:    stateMgr,
		scfg:     scfg,
		jobs:     map[abi.SectorID]struct{}{},
	}

	return committer, nil
}

type SnapUpCommitter struct {
	ctx    context.Context
	cancel context.CancelFunc

	tracker  core.SectorTracker
	indexer  core.SectorIndexer
	chain    chain.API
	eventbus *chain.EventBus
	messager messager.API
	state    core.SectorStateManager
	scfg     *modules.SafeConfig

	jobs     map[abi.SectorID]struct{}
	jobsMu   sync.Mutex
	jobsOnce sync.Once
}

func (sc *SnapUpCommitter) Start() error {
	var err error
	sc.jobsOnce.Do(func() {
		var count int
		sc.jobsMu.Lock()
		defer sc.jobsMu.Unlock()

		err = sc.state.ForEach(sc.ctx, core.WorkerOnline, core.SectorWorkerJobSnapUp, func(state core.SectorState) error {
			if !state.Upgraded {
				return nil
			}

			sc.jobs[state.ID] = struct{}{}
			go sc.commitSector(state)
			count++
			return nil
		})

		snapupLog.Infow("snapup sectors loaded", "count", count)
	})

	return err
}

func (sc *SnapUpCommitter) Stop() {
	sc.cancel()
	snapupLog.Info("stopped")
}

func (sc *SnapUpCommitter) Commit(ctx context.Context, sid abi.SectorID) error {
	sc.jobsMu.Lock()
	_, exist := sc.jobs[sid]
	if !exist {
		sc.jobs[sid] = struct{}{}
	}
	sc.jobsMu.Unlock()

	if exist {
		return fmt.Errorf("duplicate commit job")
	}

	state, err := sc.state.Load(ctx, sid)
	if err != nil {
		return fmt.Errorf("load sector state: %w", err)
	}

	if !state.Upgraded {
		return fmt.Errorf("sector is not upgraded")
	}

	go sc.commitSector(*state)
	return nil
}

func (sc *SnapUpCommitter) commitSector(state core.SectorState) {
	defer func() {
		sc.jobsMu.Lock()
		delete(sc.jobs, state.ID)
		sc.jobsMu.Unlock()
	}()

	slog := snapupLog.With("sector", util.FormatSectorID(state.ID))
	maddr, err := address.NewIDAddress(uint64(state.ID.Miner))
	if err != nil {
		slog.Error("invalid miner actor id")
		return
	}

	ssize, err := state.SectorType.SectorSize()
	if err != nil {
		slog.Errorf("invalid sector size: %s", err)
		return
	}

	if state.UpgradedInfo == nil {
		slog.Debug("not completed yet")
		return
	}

	if err := sc.indexer.Upgrade().Update(sc.ctx, state.ID, state.UpgradedInfo.AccessInstance); err != nil {
		slog.Errorf("failed to update upgrade indexer: %s", err)
		return
	}

	handler := &snapupCommitHandler{
		maddr:     maddr,
		state:     state,
		ssize:     ssize,
		committer: sc,
	}

	timer := time.NewTimer(time.Second)
	for {
		wait := 30 * time.Second
		select {
		case <-sc.ctx.Done():
			slog.Info("committer context done")
			return

		case <-timer.C:
			finished, err := handler.handle()
			if err == nil {
				if finished {
					return
				}
			} else {
				var tempErr snapupCommitTempError
				isTemp := errors.As(err, &tempErr)
				if !isTemp {
					slog.Errorf("permanent error: %s", err)
					return
				}

				if tempErr.retry > 0 {
					wait = tempErr.retry
				}

				slog.Warnf("temporary error: %s, retry after %s", tempErr.err, wait)
			}

			timer.Reset(wait)
		}
	}
}

var errMsgNotLanded = fmt.Errorf("msg not landed")
var errMsgNoReceipt = fmt.Errorf("msg without receipt")
var errMsgPending = fmt.Errorf("pending msg")

func newTempErr(err error, retryAfter time.Duration) snapupCommitTempError {
	te := snapupCommitTempError{
		err:   err,
		retry: retryAfter,
	}

	return te
}

type snapupCommitTempError struct {
	err   error
	retry time.Duration
}

func (e snapupCommitTempError) Error() string {
	return e.err.Error()
}

func (e snapupCommitTempError) Unwrap() error {
	return e.err
}

type snapupCommitHandler struct {
	maddr     address.Address
	state     core.SectorState
	ssize     abi.SectorSize
	committer *SnapUpCommitter
}

func (h *snapupCommitHandler) handle() (bool, error) {
	if h.state.UpgradePublic == nil {
		return false, fmt.Errorf("public info missing")
	}

	if h.state.UpgradedInfo == nil {
		return false, fmt.Errorf("sector still in progress")
	}

	// handle landed upgrade sector
	if h.state.UpgradeLandedEpoch != nil {
		err := h.landed()
		return err == nil, err
	}

	// handle submitted upgrade sector
	if h.state.UpgradeMessageID != nil {
		err := h.waitForMessage()
		return false, err
	}

	return false, h.submitMessage()
}

func (h *snapupCommitHandler) checkUpgradeInfo() error {
	// TODO: check more methods
	return nil
}

func (h *snapupCommitHandler) submitMessage() error {
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}

	if !mcfg.SnapUp.Sender.Valid() {
		return fmt.Errorf("snapup sender address invalid")
	}

	if !mcfg.SnapUp.Enabled {
		return fmt.Errorf("snapup disabled")
	}

	if err := h.checkUpgradeInfo(); err != nil {
		return err
	}

	ts, err := h.committer.chain.ChainHead(h.committer.ctx)
	if err != nil {
		return newTempErr(fmt.Errorf("get chain head: %w", err), time.Minute)
	}

	tsk := ts.Key()

	sl, err := h.committer.chain.StateSectorPartition(h.committer.ctx, h.maddr, h.state.ID.Number, tsk)
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	}
	updateProof, err := h.state.SectorType.RegisteredUpdateProof()
	if err != nil {
		return fmt.Errorf("get registered update proof type: %w", err)
	}

	enc := new(bytes.Buffer)
	params := &miner.ProveReplicaUpdatesParams{
		Updates: []miner.ReplicaUpdate{
			{
				SectorID:           h.state.ID.Number,
				Deadline:           sl.Deadline,
				Partition:          sl.Partition,
				NewSealedSectorCID: h.state.UpgradedInfo.SealedCID,
				Deals:              h.state.DealIDs(),
				UpdateProofType:    updateProof,
				ReplicaProof:       h.state.UpgradedInfo.Proof,
			},
		},
	}

	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("serialize params: %w", err)
	}

	msg := types.Message{
		From:   mcfg.SnapUp.Sender.Std(),
		To:     h.maddr,
		Method: miner.Methods.ProveReplicaUpdates,
		Params: enc.Bytes(),
		Value:  types.NewInt(0),
	}

	spec := messager.MsgMeta{
		GasOverEstimation: mcfg.SnapUp.GasOverEstimation,
		MaxFeeCap:         mcfg.SnapUp.MaxFeeCap.Std(),
	}

	mcid := msg.Cid().String()
	has, err := h.committer.messager.HasMessageByUid(h.committer.ctx, mcid)
	if err != nil {
		return newTempErr(fmt.Errorf("check if message exists: %w", err), time.Minute)
	}

	if !has {
		uid, err := h.committer.messager.PushMessageWithId(h.committer.ctx, mcid, &msg, &spec)
		if err != nil {
			return newTempErr(fmt.Errorf("push ProveReplicaUpdates message: %w", err), time.Minute)
		}

		mcid = uid
	}

	msgID := core.SectorUpgradeMessageID(mcid)
	if err := h.committer.state.Update(h.committer.ctx, h.state.ID, &msgID); err != nil {
		return newTempErr(fmt.Errorf("update UpgradeMessageID: %w", err), time.Minute)
	}

	h.state.UpgradeMessageID = &msgID

	return nil
}

func (h *snapupCommitHandler) waitForMessage() error {
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}

	// TODO: similar handling in commitmgr
	msg, err := h.committer.messager.GetMessageByUid(h.committer.ctx, string(*h.state.UpgradeMessageID))
	if err != nil {
		return newTempErr(err, time.Minute)
	}

	var maybeMsg string
	if msg.State != messager.MessageState.OnChainMsg && msg.Receipt != nil && len(msg.Receipt.Return) > 0 {
		maybeMsg = string(msg.Receipt.Return)
	}

	switch msg.State {
	case messager.MessageState.OnChainMsg:
		if msg.Confidence < int64(mcfg.SnapUp.MessageConfidential) {
			return newTempErr(errMsgNotLanded, time.Minute)
		}

		if msg.Receipt == nil {
			return newTempErr(errMsgNoReceipt, time.Minute)
		}

		if msg.Receipt.ExitCode != exitcode.Ok {
			return fmt.Errorf("failed on-chain message with exitcode=%s, error=%q", msg.Receipt.ExitCode, maybeMsg)
		}

		if msg.TipSetKey.IsEmpty() {
			return newTempErr(fmt.Errorf("get empty tipset key"), time.Minute)
		}

	case messager.MessageState.FailedMsg:
		return fmt.Errorf("failed off-chain message with error=%q", maybeMsg)

	default:
		return newTempErr(errMsgPending, time.Minute)
	}

	ts, err := h.committer.chain.ChainGetTipSet(h.committer.ctx, msg.TipSetKey)
	if err != nil {
		return newTempErr(fmt.Errorf("get tipset %q: %w", msg.TipSetKey.String(), err), time.Minute)
	}

	landedEpoch := core.SectorUpgradeLandedEpoch(ts.Height())
	err = h.committer.state.Update(h.committer.ctx, h.state.ID, &landedEpoch)
	if err != nil {
		return newTempErr(fmt.Errorf("update sector state: %w", err), time.Minute)
	}

	h.state.UpgradeLandedEpoch = &landedEpoch
	return nil
}

func (h *snapupCommitHandler) landed() error {
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}

	errChan := make(chan error, 1)
	h.committer.eventbus.At(h.committer.ctx, abi.ChainEpoch(*h.state.UpgradeLandedEpoch)+policy.ChainFinality, mcfg.SnapUp.ReleaseCondidential, func(ts *types.TipSet) {
		defer close(errChan)
		if err := h.cleanupForSector(); err != nil {
			errChan <- fmt.Errorf("cleanup data before upgrading: %w", err)
			return
		}

		if err := h.committer.state.Finalize(h.committer.ctx, h.state.ID, nil); err != nil {
			errChan <- fmt.Errorf("finalize sector: %w", err)
			return
		}
	})

	select {
	case <-h.committer.ctx.Done():
		return h.committer.ctx.Err()

	case err := <-errChan:
		if err != nil {
			return newTempErr(err, 0)
		}

		return nil
	}

}

func (h *snapupCommitHandler) cleanupForSector() error {
	sref := core.SectorRef{
		ID:        h.state.ID,
		ProofType: h.state.SectorType,
	}

	privateInfo, err := h.committer.tracker.SinglePrivateInfo(h.committer.ctx, sref, false, nil)
	if err != nil {
		return fmt.Errorf("get private info from tracker: %w", err)
	}

	fileURIs := util.CachedFilesForSectorSize(privateInfo.CacheDirURI, h.ssize)
	fileURIs = append(fileURIs, privateInfo.SealedSectorURI)

	store, err := h.committer.indexer.StoreMgr().GetInstance(h.committer.ctx, privateInfo.AccessInstance)
	if err != nil {
		return fmt.Errorf("get store instance %s: %w", privateInfo.AccessInstance, err)
	}

	var errwg multierror.Group
	for fi := range fileURIs {
		uri := fileURIs[fi]
		errwg.Go(func() error {
			delErr := store.Del(h.committer.ctx, uri)
			if delErr == nil {
				return nil
			}

			if errors.Is(delErr, objstore.ErrObjectNotFound) {
				return nil
			}

			return fmt.Errorf("attempt to del obj %q: %w", uri, err)
		})
	}

	merr := errwg.Wait().ErrorOrNil()
	if merr != nil {
		return newTempErr(merr, time.Minute)
	}

	return nil
}
