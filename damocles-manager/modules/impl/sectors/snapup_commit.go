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
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/hashicorp/go-multierror"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	mpolicy "github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util/piece"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util/pledge"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
)

//revive:disable-next-line:argument-limit
func NewSnapUpCommitter(
	ctx context.Context,
	tracker core.SectorTracker,
	indexer core.SectorIndexer,
	chainAPI chain.API,
	eventbus *chain.EventBus,
	messagerAPI messager.API,
	stateMgr core.SectorStateManager,
	scfg *modules.SafeConfig,
	lookupID core.LookupID,
	senderSelector core.SenderSelector,
) (*SnapUpCommitter, error) {
	ctx, cancel := context.WithCancel(ctx)
	committer := &SnapUpCommitter{
		ctx:            ctx,
		cancel:         cancel,
		tracker:        tracker,
		indexer:        indexer,
		chain:          chainAPI,
		eventbus:       eventbus,
		messager:       messagerAPI,
		state:          stateMgr,
		senderSelector: senderSelector,
		scfg:           scfg,
		lookupID:       lookupID,
		jobs:           map[abi.SectorID]context.CancelFunc{},
	}

	return committer, nil
}

type SnapUpCommitter struct {
	ctx    context.Context
	cancel context.CancelFunc

	tracker        core.SectorTracker
	indexer        core.SectorIndexer
	chain          chain.API
	eventbus       *chain.EventBus
	messager       messager.API
	state          core.SectorStateManager
	senderSelector core.SenderSelector
	scfg           *modules.SafeConfig
	lookupID       core.LookupID

	jobs     map[abi.SectorID]context.CancelFunc
	jobsMu   sync.Mutex
	jobsOnce sync.Once
}

func (sc *SnapUpCommitter) Start() error {
	var err error
	sc.jobsOnce.Do(func() {
		var count int
		sc.jobsMu.Lock()
		defer sc.jobsMu.Unlock()

		err = sc.state.ForEach(
			sc.ctx,
			core.WorkerOnline,
			core.SectorWorkerJobSnapUp,
			func(state core.SectorState) error {
				if !state.Upgraded {
					return nil
				}

				ctx, cancel := context.WithCancel(sc.ctx)
				sc.jobs[state.ID] = cancel
				go sc.commitSector(ctx, state)
				count++
				return nil
			},
		)

		snapupLog.Infow("snapup sectors loaded", "count", count)
	})

	return err
}

func (sc *SnapUpCommitter) Stop() {
	sc.cancel()
	snapupLog.Info("stopped")
}

func (sc *SnapUpCommitter) Commit(ctx context.Context, sid abi.SectorID) error {
	var jobCtx context.Context
	sc.jobsMu.Lock()
	_, exist := sc.jobs[sid]
	if !exist {
		var jobCancel context.CancelFunc
		jobCtx, jobCancel = context.WithCancel(sc.ctx)
		sc.jobs[sid] = jobCancel
	}
	sc.jobsMu.Unlock()

	if exist {
		return fmt.Errorf("duplicate commit job")
	}

	state, err := sc.state.Load(ctx, sid, core.WorkerOnline)
	if err != nil {
		return fmt.Errorf("load sector state: %w", err)
	}

	if !state.Upgraded {
		return fmt.Errorf("sector is not upgraded")
	}

	go sc.commitSector(jobCtx, *state)
	return nil
}

func (sc *SnapUpCommitter) CancelCommitment(_ context.Context, sid abi.SectorID) {
	sc.jobsMu.Lock()
	defer sc.jobsMu.Unlock()

	cancel, exist := sc.jobs[sid]
	if !exist {
		return
	}

	delete(sc.jobs, sid)
	cancel()
}

func (sc *SnapUpCommitter) commitSector(ctx context.Context, state core.SectorState) {
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

	if err := sc.indexer.Upgrade().Update(sc.ctx, state.ID, core.SectorAccessStores{
		SealedFile: state.UpgradedInfo.AccessInstance,
		CacheDir:   state.UpgradedInfo.AccessInstance,
	}); err != nil {
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
	retried := 0
	for {
		wait := 30 * time.Second
		select {
		case <-ctx.Done():
			slog.Info("commit job context done")
			return

		case <-sc.ctx.Done():
			slog.Info("committer context done")
			return

		case <-timer.C:
			finished, err := handler.handle()
			if err == nil {
				if finished {
					return
				}

				retried = 0
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

				slog.Warnf("temporary error: %s, %d attempts done, retry after %s", tempErr.err, retried, wait)

				mcfg, err := sc.scfg.MinerConfig(state.ID.Miner)
				if err != nil {
					slog.Errorf("get miner config for %d: %s", state.ID.Miner, err)
					return
				}

				if max := mcfg.SnapUp.Retry.MaxAttempts; max != nil && retried >= *max {
					slog.Warn("max retry attempts exceeded, abort current submit")
					return
				}

				retried++
			}

			timer.Reset(wait)
		}
	}
}

var (
	errMsgNotLanded = fmt.Errorf("msg not landed")
	errMsgNoReceipt = fmt.Errorf("msg without receipt")
	errMsgPending   = fmt.Errorf("pending msg")
	errMsgTempErr   = fmt.Errorf("msg temp error")
)

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

func (*snapupCommitHandler) checkUpgradeInfo() error {
	// TODO: check more methods
	return nil
}

func (h *snapupCommitHandler) submitMessage() error {
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}

	if !mcfg.SnapUp.Enabled {
		return fmt.Errorf("snapup disabled")
	}

	sender, err := h.committer.senderSelector.Select(h.committer.ctx, h.state.ID.Miner, mcfg.PoSt.GetSenders())
	if err != nil {
		return fmt.Errorf("select sender for %d: %w", h.state.ID.Miner, err)
	}

	if err := h.checkUpgradeInfo(); err != nil {
		return err
	}

	ts, err := h.committer.chain.ChainHead(h.committer.ctx)
	if err != nil {
		return newTempErr(fmt.Errorf("get chain head: %w", err), mcfg.SnapUp.Retry.APIFailureWait.Std())
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

	currDeadline, err := h.committer.chain.StateMinerProvingDeadline(h.committer.ctx, h.maddr, tsk)
	if err != nil {
		return newTempErr(fmt.Errorf("get proving deadline: %w", err), mcfg.SnapUp.Retry.APIFailureWait.Std())
	}

	// If the deadline is the current or next deadline to prove, don't allow updating sectors.
	// We assume that deadlines are immutable when being proven.
	//
	// `abi.ChainEpoch(10)` indicates that we assume that the message will be real executed within 10 heights
	//nolint:all
	// See: https://github.com/filecoin-project/builtin-actors/blob/10f547c950a99a07231c08a3c6f4f76ff0080a7c/actors/miner/src/lib.rs#L1113-L1124
	if isMut, delayBlock := deadlineIsMutable(currDeadline.PeriodStart, sl.Deadline, ts.Height(), abi.ChainEpoch(10)); !isMut { //revive:disable-line:line-length-limit
		delayTime := time.Duration(mpolicy.NetParams.BlockDelaySecs*uint64(delayBlock)) * time.Second
		return newTempErr(
			fmt.Errorf(
				"cannot upgrade sectors in immutable deadline: %d. sector: %s",
				sl.Deadline,
				util.FormatSectorID(h.state.ID),
			),
			delayTime,
		)
	}

	pams, _, err := piece.ProcessPieces(
		h.committer.ctx,
		&h.state,
		h.committer.chain,
		h.committer.lookupID,
	)
	if err != nil {
		return newTempErr(fmt.Errorf("failed to process pieces: %w", err), mcfg.SnapUp.Retry.APIFailureWait.Std())
	}

	enc := new(bytes.Buffer)
	params := &miner.ProveReplicaUpdates3Params{
		SectorUpdates: []miner.SectorUpdateManifest{
			{
				Sector:       h.state.ID.Number,
				Deadline:     sl.Deadline,
				Partition:    sl.Partition,
				NewSealedCID: h.state.UpgradedInfo.SealedCID,
				Pieces:       pams,
			},
		},
		SectorProofs:     [][]byte{h.state.UpgradedInfo.Proof},
		UpdateProofsType: updateProof,
		// AggregateProof
		// AggregateProofType
		RequireActivationSuccess:   mcfg.Sealing.RequireActivationSuccessUpdate,
		RequireNotificationSuccess: mcfg.Sealing.RequireNotificationSuccessUpdate,
	}

	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("serialize params: %w", err)
	}

	msgValue := types.NewInt(0)
	if mcfg.SnapUp.SendFund {
		proofType, err := h.state.SectorType.RegisteredWindowPoStProof()
		if err != nil {
			return fmt.Errorf("get registered window post proof type: %w", err)
		}

		collateral, err := h.calcCollateral(tsk, proofType)
		if err != nil {
			return newTempErr(err, mcfg.SnapUp.Retry.APIFailureWait.Std())
		}

		msgValue = collateral
	}

	msg := types.Message{
		From:      sender,
		To:        h.maddr,
		Method:    builtin.MethodsMiner.ProveReplicaUpdates3,
		Params:    enc.Bytes(),
		Value:     msgValue,
		GasFeeCap: mcfg.SnapUp.GetGasFeeCap().Std(),
	}

	spec := mcfg.SnapUp.FeeConfig.GetSendSpec()
	mcid := msg.Cid().String()
	for i := 0; ; i++ {
		mcidTemp := fmt.Sprintf("%s-%d", mcid, i)
		has, err := h.committer.messager.HasMessageByUid(h.committer.ctx, mcidTemp)
		if err != nil {
			return newTempErr(fmt.Errorf("check if message exists: %w", err), mcfg.SnapUp.Retry.APIFailureWait.Std())
		}
		if !has {
			mcid = mcidTemp
			break
		}
	}

	uid, err := h.committer.messager.PushMessageWithId(h.committer.ctx, mcid, &msg, &spec)
	if err != nil {
		return newTempErr(
			fmt.Errorf("push ProveReplicaUpdates message: %w", err),
			mcfg.SnapUp.Retry.APIFailureWait.Std(),
		)
	}

	mcid = uid

	msgID := core.SectorUpgradeMessageID(mcid)
	if err := h.committer.state.Update(h.committer.ctx, h.state.ID, core.WorkerOnline, &msgID); err != nil {
		return newTempErr(fmt.Errorf("update UpgradeMessageID: %w", err), mcfg.SnapUp.Retry.LocalFailureWait.Std())
	}

	h.state.UpgradeMessageID = &msgID

	return nil
}

func (h *snapupCommitHandler) calcCollateral(tsk types.TipSetKey, proofType abi.RegisteredPoStProof) (big.Int, error) {
	onChainInfo, err := h.committer.chain.StateSectorGetInfo(h.committer.ctx, h.maddr, h.state.ID.Number, tsk)
	if err != nil {
		return big.Int{}, fmt.Errorf("StateSectorGetInfo: %w", err) //revive:disable-line:error-strings
	}

	nv, err := h.committer.chain.StateNetworkVersion(h.committer.ctx, tsk)
	if err != nil {
		return big.Int{}, fmt.Errorf("StateNetworkVersion: %w", err) //revive:disable-line:error-strings
	}
	// TODO: Drop after nv19 comes and goes
	if nv >= network.Version19 {
		proofType, err = proofType.ToV1_1PostProof()
		if err != nil {
			return big.Int{}, fmt.Errorf("convert to v1_1 post proof: %w", err)
		}
	}
	sealType, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, proofType, false)
	if err != nil {
		return big.Int{}, fmt.Errorf("get seal proof type: %w", err)
	}

	weightUpdate, err := pledge.SectorWeight(
		h.committer.ctx,
		&h.state,
		sealType,
		h.committer.chain,
		onChainInfo.Expiration,
	)
	if err != nil {
		return big.Int{}, fmt.Errorf("get sector weight: %w", err)
	}

	collateral, err := pledge.CalcPledgeForPower(h.committer.ctx, h.committer.chain, weightUpdate)
	if err != nil {
		return big.Int{}, fmt.Errorf("get pledge for power: %w", err)
	}
	collateral = big.Sub(collateral, onChainInfo.InitialPledge)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	return collateral, nil
}

func (h *snapupCommitHandler) waitForMessage() error {
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}

	// TODO: similar handling in commitmgr
	msgID := string(*h.state.UpgradeMessageID)
	msg, err := h.committer.messager.GetMessageByUid(h.committer.ctx, msgID)
	if err != nil {
		return newTempErr(err, mcfg.SnapUp.Retry.APIFailureWait.Std())
	}

	var maybeMsg string
	if msg.State != messager.MessageState.OnChainMsg && msg.State != messager.MessageState.NonceConflictMsg &&
		msg.Receipt != nil &&
		len(msg.Receipt.Return) > 0 {
		maybeMsg = string(msg.Receipt.Return)
	}

	switch msg.State {
	case messager.MessageState.OnChainMsg, messager.MessageState.NonceConflictMsg:
		if msg.Confidence < int64(mcfg.SnapUp.GetMessageConfidence()) {
			return newTempErr(errMsgNotLanded, mcfg.SnapUp.Retry.PollInterval.Std())
		}

		if msg.Receipt == nil {
			return newTempErr(errMsgNoReceipt, mcfg.SnapUp.Retry.PollInterval.Std())
		}

		switch msg.Receipt.ExitCode {
		case exitcode.Ok:
		case exitcode.SysErrInsufficientFunds, exitcode.SysErrOutOfGas:
			// should resend here
			h.state.UpgradeMessageID = nil
			err = h.committer.state.Update(h.committer.ctx, h.state.ID, core.WorkerOnline, &h.state)
			if err != nil {
				return newTempErr(fmt.Errorf("update sector state: %w", err), mcfg.SnapUp.Retry.LocalFailureWait.Std())
			}
			return newTempErr(fmt.Errorf("%s: %w", errMsgTempErr, err), mcfg.SnapUp.Retry.PollInterval.Std())
		default:
			return fmt.Errorf("failed on-chain message with exitcode=%s, error=%q", msg.Receipt.ExitCode, maybeMsg)
		}

		if msg.TipSetKey.IsEmpty() {
			return newTempErr(fmt.Errorf("get empty tipset key"), mcfg.SnapUp.Retry.PollInterval.Std())
		}

	case messager.MessageState.FailedMsg:
		return fmt.Errorf("failed off-chain message with error=%q", maybeMsg)

	default:
		return newTempErr(errMsgPending, mcfg.SnapUp.Retry.PollInterval.Std())
	}

	ts, err := h.committer.chain.ChainGetTipSet(h.committer.ctx, msg.TipSetKey)
	if err != nil {
		return newTempErr(
			fmt.Errorf("get tipset %q: %w", msg.TipSetKey.String(), err),
			mcfg.SnapUp.Retry.APIFailureWait.Std(),
		)
	}

	landedEpoch := core.SectorUpgradeLandedEpoch(ts.Height())
	err = h.committer.state.Update(h.committer.ctx, h.state.ID, core.WorkerOnline, &landedEpoch)
	if err != nil {
		return newTempErr(fmt.Errorf("update sector state: %w", err), mcfg.SnapUp.Retry.LocalFailureWait.Std())
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
	h.committer.eventbus.At(
		h.committer.ctx,
		abi.ChainEpoch(*h.state.UpgradeLandedEpoch)+policy.ChainFinality,
		mcfg.SnapUp.GetReleaseConfidence(),
		func(ts *types.TipSet) {
			defer close(errChan)
			if err := h.cleanupForSector(); err != nil {
				errChan <- fmt.Errorf("cleanup data before upgrading: %w", err)
				return
			}

			if err := h.committer.state.Finalize(h.committer.ctx, h.state.ID, nil); err != nil {
				errChan <- fmt.Errorf("finalize sector: %w", err)
				return
			}
		},
	)

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
	mcfg, err := h.committer.scfg.MinerConfig(h.state.ID.Miner)
	if err != nil {
		return fmt.Errorf("get miner config: %w", err)
	}
	if !mcfg.SnapUp.CleanupCCData {
		log.Debug("skip cleanup CC data")
		return nil
	}
	sref := core.SectorRef{
		ID:        h.state.ID,
		ProofType: h.state.SectorType,
	}

	privateInfo, err := h.committer.tracker.SinglePrivateInfo(h.committer.ctx, sref, false, nil)
	if err != nil {
		return fmt.Errorf("get private info from tracker: %w", err)
	}

	cleanupTargets := []struct {
		storeInstance string
		fileURIs      []string
	}{
		{
			storeInstance: privateInfo.Accesses.SealedFile,
			fileURIs:      []string{privateInfo.SealedSectorURI},
		},
		{
			storeInstance: privateInfo.Accesses.CacheDir,
			fileURIs:      util.CachedFilesForSectorSize(privateInfo.CacheDirURI, h.ssize),
		},
	}

	for ti := range cleanupTargets {
		storeInstance := cleanupTargets[ti].storeInstance
		store, err := h.committer.indexer.StoreMgr().GetInstance(h.committer.ctx, storeInstance)
		if err != nil {
			return fmt.Errorf("get store instance %s: %w", storeInstance, err)
		}

		fileURIs := cleanupTargets[ti].fileURIs
		var errwg multierror.Group
		for fi := range fileURIs {
			uri := fileURIs[fi]
			errwg.Go(func() error {
				delErr := store.Del(h.committer.ctx, uri)
				if delErr == nil {
					log.Debugf("CC data cleaned: %s, store: %s", uri, storeInstance)
					return nil
				}

				if errors.Is(delErr, objstore.ErrObjectNotFound) {
					return nil
				}

				return fmt.Errorf("attempt to del obj %q: %w", uri, delErr)
			})
		}

		merr := errwg.Wait().ErrorOrNil()
		if merr != nil {
			return newTempErr(merr, mcfg.SnapUp.Retry.LocalFailureWait.Std())
		}
	}

	return nil
}

// Returns true if the deadline at the given index is currently mutable.
func deadlineIsMutable(
	provingPeriodStart abi.ChainEpoch,
	deadlineIdx uint64,
	currentEpoch, msgExecInterval abi.ChainEpoch,
) (bool, abi.ChainEpoch) {
	// Get the next non-elapsed deadline (i.e., the next time we care about
	// mutations to the deadline).
	deadlineInfo := stminer.NewDeadlineInfo(provingPeriodStart, deadlineIdx, currentEpoch).NextNotElapsed()

	// Ensure that the current epoch is at least one challenge window before
	// that deadline opens.
	var delay abi.ChainEpoch
	isMut := currentEpoch < deadlineInfo.Open-stminer.WPoStChallengeWindow-msgExecInterval

	if !isMut { // deadline is immutable. should delay
		delay = deadlineInfo.Close - currentEpoch
		log.Warnf(
			"delay upgrade to avoid mutating deadline %d at %d before it opens at %d",
			deadlineIdx,
			currentEpoch,
			deadlineInfo.Open,
		)
	}

	return isMut, delay
}
