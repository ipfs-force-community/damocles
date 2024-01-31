package poster

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v12/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
)

type HeadChangeSubscriber func(revert, apply *types.TipSet)

const (
	DefaultSubmitConfidence    = 4
	DefaultChallengeConfidence = 10
)

var log = logging.New("poster")

func newPostDeps(
	chain chain.API,
	msg messager.API,
	rand core.RandomnessAPI,
	minerAPI core.MinerAPI,
	prover core.Prover,
	verifier core.Verifier,
	sectorProving core.SectorProving,
	senderSelect core.SenderSelector,
) postDeps {
	return postDeps{
		chain:          chain,
		msg:            msg,
		rand:           rand,
		minerAPI:       minerAPI,
		clock:          clock.NewSystemClock(),
		prover:         prover,
		verifier:       verifier,
		sectorProving:  sectorProving,
		senderSelector: senderSelect,
	}
}

type postDeps struct {
	chain          chain.API
	msg            messager.API
	rand           core.RandomnessAPI
	minerAPI       core.MinerAPI
	clock          clock.Clock
	prover         core.Prover
	verifier       core.Verifier
	sectorProving  core.SectorProving
	senderSelector core.SenderSelector
}

func NewPoSter(
	scfg *modules.SafeConfig,
	chain chain.API,
	msg messager.API,
	rand core.RandomnessAPI,
	minerAPI core.MinerAPI,
	prover core.Prover,
	verifier core.Verifier,
	sectorProving core.SectorProving,
	senderSelector core.SenderSelector,
) (*PoSter, error) {
	return &PoSter{
		cfg:  scfg,
		deps: newPostDeps(chain, msg, rand, minerAPI, prover, verifier, sectorProving, senderSelector),
	}, nil
}

type PoSter struct {
	cfg  *modules.SafeConfig
	deps postDeps
}

func (p *PoSter) Run(ctx context.Context) {
	var notifs <-chan []*chain.HeadChange
	firstTime := true
	reconnectWait := 10 * time.Second
	mlog := log.With("mod", "notify")

	subscribers := make(map[abi.ActorID][]HeadChangeSubscriber)

	// not fine to panic after this point
CHAIN_HEAD_LOOP:
	for {
		if notifs == nil {
			if !firstTime {
				mlog.Warnf("try to reconnect after %s", reconnectWait)
				select {
				case <-ctx.Done():
					return

				case <-time.After(reconnectWait):

				}

			} else {
				firstTime = false
			}

			ch, err := p.deps.chain.ChainNotify(ctx)
			if err != nil {
				mlog.Errorf("get ChainNotify error: %s", err)
				continue CHAIN_HEAD_LOOP
			}

			if ch == nil {
				mlog.Error("get nil channel")
				continue CHAIN_HEAD_LOOP
			}

			mlog.Debug("channel established")
			notifs = ch
		}

		select {
		case <-ctx.Done():
			return

		case changes, ok := <-notifs:
			if !ok {
				mlog.Warn("channel closed")
				notifs = nil
				continue CHAIN_HEAD_LOOP
			}

			var lowest, highest *types.TipSet = nil, nil
			if len(changes) == 1 && changes[0].Type == chain.HCCurrent {
				highest = changes[0].Val
			} else {
				for _, change := range changes {
					if change.Val == nil {
						mlog.Warnw("change with nil Val", "type", change.Type)
						continue
					}

					switch change.Type {
					case chain.HCRevert:
						lowest = change.Val
					case chain.HCApply:
						highest = change.Val
					}
				}
			}

			if lowest == nil && highest == nil {
				continue CHAIN_HEAD_LOOP
			}

			mlog.Debugw("chain notify get change", "len", len(changes), "lowest", lowest.Height(), "highest", highest.Height())

			mids := p.getdMiners()
			for _, mid := range mids {
				if _, ok := subscribers[mid]; !ok {
					// get all dl info
					dinfos, err := p.getAllDeadlineInfo(ctx, mid, highest)
					if err != nil {
						mlog.Errorf("get deadline info fail", "miner", mid, "err", err)
						continue
					}

					maddr, err := address.NewIDAddress(uint64(mid))
					if err != nil {
						mlog.Errorf("construct id address fail", "miner", mid, "err", err)
						continue
					}

					minfo, err := p.deps.minerAPI.GetInfo(ctx, mid)
					if err != nil {
						mlog.Errorf("get miner info: %w", err)
						continue
					}

					// create subscriber for dl info
					subs := make([]HeadChangeSubscriber, 0, len(dinfos))
					for _, dinfo := range dinfos {
						sub, _ := StartWdPostScheduler(ctx, p.deps, mid, maddr, minfo.WindowPoStProofType, dinfo, p.cfg)
						subs = append(subs, sub)
					}
					subscribers[mid] = subs
				}
			}

			for _, subs := range subscribers {
				for _, sub := range subs {
					sub(lowest, highest)
				}
			}
		}
	}
}

func (p *PoSter) getdMiners() []abi.ActorID {
	p.cfg.Lock()
	defer p.cfg.Unlock()
	mids := make([]abi.ActorID, 0, len(p.cfg.Miners))
	for _, mcfg := range p.cfg.Miners {
		mids = append(mids, mcfg.Actor)
	}
	return mids
}

func (p *PoSter) getAllDeadlineInfo(ctx context.Context, mids abi.ActorID, ts *types.TipSet) ([]*dline.Info, error) {
	tsk := ts.Key()
	tsh := ts.Height()
	ret := make([]*dline.Info, 0, 48)

	maddr, err := address.NewIDAddress(uint64(mids))
	if err != nil {
		return nil, fmt.Errorf("construct id address: %w", err)
	}

	currentDlInfo, err := p.deps.chain.StateMinerProvingDeadline(ctx, maddr, tsk)
	if err != nil {
		return nil, fmt.Errorf("rpc call: %w", err)
	}

	for dlIdx := uint64(0); dlIdx < currentDlInfo.WPoStPeriodDeadlines; dlIdx++ {
		dinfo := miner.NewDeadlineInfo(currentDlInfo.PeriodStart, dlIdx, tsh).NextNotElapsed()
		ret = append(ret, dinfo)
	}

	return ret, nil
}

func StartWdPostScheduler(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.SafeConfig) (HeadChangeSubscriber, func() Scheduler) {
	innerCtx, cancel := context.WithCancel(ctx)
	var machine Scheduler

	newStateMachine := func(ctx context.Context, height abi.ChainEpoch, cfg *modules.MinerPoStConfig) Scheduler {
		dinfo = miner.NewDeadlineInfo(dinfo.PeriodStart, dinfo.Index, height).NextNotElapsed()
		executor := NewPostExecutor(ctx, deps, mid, maddr, proofType, dinfo, cfg)

		return iniStateMachine(ctx, executor, dinfo, cfg)
	}

	subscriber := func(revert, apply *types.TipSet) {
		log := slog.With("revert", revert.Height(), "apply", apply.Height(), "deadline", dinfo.Index, "challenge", dinfo.Challenge, "open", dinfo.Open, "close", dinfo.Close)
		log.Debugf("apply begin")
		start := time.Now()
		defer func() {
			log.Debugf("apply end, elapsed: %s", time.Since(start))
		}()

		cfg.Lock()
		mcfg, err := cfg.MinerConfig(mid)
		cfg.Unlock()
		if err != nil {
			log.Errorf("get miner config of %s : %w", mid, err)
			return
		}
		pcfg := mcfg.PoSt

		if err != nil {
			log.Errorf("get post config of %s : %w", mid, err)
			return
		}
		if !pcfg.Enabled {
			return
		}

		ts := apply
		height := ts.Height()
		if machine == nil {
			machine = newStateMachine(innerCtx, height, &pcfg)
		}

		// check revert and apply
		revertAheadChallenge := revert != nil && revert.Height() < dinfo.Challenge
		applyExceedClose := apply.Height() > dinfo.Close
		if revertAheadChallenge || applyExceedClose {
			// rest machine
			cancel()
			innerCtx, cancel = context.WithCancel(ctx)
			machine = newStateMachine(innerCtx, height, &pcfg)

			if revertAheadChallenge {
				log.Warnf("revert ahead challenge: %d < %d", revert.Height(), dinfo.Challenge)
			} else {
				log.Warnf("apply exceed close: %d > %d", apply.Height(), dinfo.Close)
			}
			return
		}

		tick := &EventBody{
			Type: EventTick,
			Data: ts,
		}
		machine.Apply(tick)
	}

	runnerGetter := func() Scheduler {
		return machine
	}

	return subscriber, runnerGetter
}

const (
	// state
	stateWaitForChallenge State = "wait_for_challenge"
	statePreparing        State = "preparing"
	stateProofGenerating  State = "generating"
	stateWaitForOpen      State = "wait_for_open"
	stateSubmitting       State = "submitting"
	stateSubmitted        State = "submitted"
)

func iniStateMachine(ctx context.Context, executor postExecutor, dinfo *dline.Info, cfg *modules.MinerPoStConfig) Scheduler {
	mc := &Machine{
		logger:   log.With("deadline", dinfo.Index, "challenge", dinfo.Challenge, "open", dinfo.Open, "close", dinfo.Close),
		errRetry: int(cfg.RetryNum),
	}

	var proofs []miner.SubmitWindowedPoStParams
	var prepareResult *PrepareResult

	mc.RegisterHandler(StateEmpty, EventTick, func(data any) (State, Event, error) {
		return stateWaitForChallenge, nil, nil
	})

	mc.RegisterHandler(stateWaitForChallenge, EventTick, func(data any) (State, Event, error) {
		ts, ok := data.(*types.TipSet)
		if !ok {
			return "", nil, fmt.Errorf("invalid data of tick event")
		}

		height := ts.Height()
		challengeConfidence := cfg.ChallengeConfidence
		if challengeConfidence == 0 {
			challengeConfidence = DefaultChallengeConfidence
		}
		if height < dinfo.Challenge+abi.ChainEpoch(challengeConfidence) {
			return "", nil, nil
		}

		// trigger declare fault and recovery
		go executor.HandleFaults(ts)

		return statePreparing, nil, nil
	})

	mc.RegisterHandler(statePreparing, EventTick, func(data any) (State, Event, error) {
		res, err := executor.Prepare()
		if err != nil {
			return "", nil, err
		}
		prepareResult = res
		return stateProofGenerating, nil, nil
	})

	mc.RegisterHandler(stateProofGenerating, EventTick, func(data any) (State, Event, error) {
		var err error
		proofs, err = executor.GeneratePoSt(prepareResult)
		if err != nil {
			return "", nil, err
		}
		return stateWaitForOpen, nil, nil
	})

	mc.RegisterHandler(stateWaitForOpen, EventTick, func(data any) (State, Event, error) {
		if len(proofs) == 0 {
			return stateProofGenerating, nil, fmt.Errorf("proofs is empty")
		}

		ts, ok := data.(*types.TipSet)
		if !ok {
			return "", nil, fmt.Errorf("invalid data of tick event")
		}

		if ts.Height() > dinfo.Open {
			return stateSubmitting, nil, nil
		}

		return "", nil, nil
	})

	mc.RegisterHandler(stateSubmitting, EventTick, func(data any) (State, Event, error) {
		ts, ok := data.(*types.TipSet)
		if !ok {
			return "", nil, fmt.Errorf("invalid data of tick event")
		}

		// submit proofs
		err := executor.SubmitPoSts(ts, proofs)
		if err != nil {
			return "", nil, err
		}

		return stateSubmitted, nil, nil
	})

	mc.Start(ctx)
	return mc
}
