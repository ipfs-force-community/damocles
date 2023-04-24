package poster

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

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
	sectorTracker core.SectorTracker,
) postDeps {
	return postDeps{
		chain:         chain,
		msg:           msg,
		rand:          rand,
		minerAPI:      minerAPI,
		clock:         clock.NewSystemClock(),
		prover:        prover,
		verifier:      verifier,
		sectorTracker: sectorTracker,
	}
}

type postDeps struct {
	chain         chain.API
	msg           messager.API
	rand          core.RandomnessAPI
	minerAPI      core.MinerAPI
	clock         clock.Clock
	prover        core.Prover
	verifier      core.Verifier
	sectorTracker core.SectorTracker
}

func NewPoSter(
	scfg *modules.SafeConfig,
	chain chain.API,
	msg messager.API,
	rand core.RandomnessAPI,
	minerAPI core.MinerAPI,
	prover core.Prover,
	verifier core.Verifier,
	sectorTracker core.SectorTracker,
) (*PoSter, error) {
	return newPoSterWithRunnerConstructor(
		scfg,
		chain,
		msg,
		rand,
		minerAPI,
		prover,
		verifier,
		sectorTracker,
		postRunnerConstructor,
	)
}

func newPoSterWithRunnerConstructor(
	scfg *modules.SafeConfig,
	chain chain.API,
	msg messager.API,
	rand core.RandomnessAPI,
	minerAPI core.MinerAPI,
	prover core.Prover,
	verifier core.Verifier,
	sectorTracker core.SectorTracker,
	runnerCtor runnerConstructor,
) (*PoSter, error) {
	return &PoSter{
		cfg:               scfg,
		deps:              newPostDeps(chain, msg, rand, minerAPI, prover, verifier, sectorTracker),
		schedulers:        make(map[abi.ActorID]map[abi.ChainEpoch]*scheduler),
		runnerConstructor: runnerCtor,
	}, nil
}

type PoSter struct {
	cfg  *modules.SafeConfig
	deps postDeps

	schedulers        map[abi.ActorID]map[abi.ChainEpoch]*scheduler
	runnerConstructor runnerConstructor
}

func (p *PoSter) Run(ctx context.Context) {
	var notifs <-chan []*chain.HeadChange
	firstTime := true

	reconnectWait := 10 * time.Second

	mlog := log.With("mod", "notify")

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

			mlog.Debug("chain notify get change", changes)

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

			mids := p.getEnabledMiners(mlog)

			if len(mids) == 0 {
				continue CHAIN_HEAD_LOOP
			}

			dinfos := p.fetchMinerProvingDeadlineInfos(ctx, mids, highest)
			if len(dinfos) == 0 {
				continue CHAIN_HEAD_LOOP
			}

			p.handleHeadChange(ctx, lowest, highest, dinfos)
		}
	}
}

func (p *PoSter) getEnabledMiners(mlog *logging.ZapLogger) []abi.ActorID {
	p.cfg.Lock()
	mids := make([]abi.ActorID, 0, len(p.cfg.Miners))
	for mi := range p.cfg.Miners {
		if mcfg := p.cfg.Miners[mi]; mcfg.PoSt.Enabled {
			if !mcfg.PoSt.Sender.Valid() {
				mlog.Warnw("post is enabled, but sender is invalid", "miner", mcfg.Actor)
				continue
			}

			mids = append(mids, mcfg.Actor)
		}
	}
	p.cfg.Unlock()

	return mids
}

func (p *PoSter) fetchMinerProvingDeadlineInfos(ctx context.Context, mids []abi.ActorID, ts *types.TipSet) map[abi.ActorID]map[abi.ChainEpoch]*dline.Info {
	count := len(mids)
	infos := make([]*dline.Info, count)
	errs := make([]error, count)

	tsk := ts.Key()
	tsh := ts.Height()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()

			maddr, err := address.NewIDAddress(uint64(mids[idx]))
			if err != nil {
				errs[idx] = fmt.Errorf("construct id address: %w", err)
				return
			}

			dinfo, err := p.deps.chain.StateMinerProvingDeadline(ctx, maddr, tsk)
			if err != nil {
				errs[idx] = fmt.Errorf("rpc call: %w", err)
				return
			}

			infos[idx] = dinfo
		}(i)
	}

	wg.Wait()

	res := map[abi.ActorID]map[abi.ChainEpoch]*dline.Info{}
	for i := range mids {
		if err := errs[i]; err != nil {
			log.Warnf("fetch proving deadline for %d: %s", mids[i], err)
			continue
		}

		dl := infos[i]
		dls := map[abi.ChainEpoch]*dline.Info{
			dl.Open: dl,
		}

		maybeNext := nextDeadline(dl, tsh)
		if deadlineIsActive(maybeNext, tsh) {
			dls[maybeNext.Open] = maybeNext
		}

		res[mids[i]] = dls
	}

	return res
}

// handleHeadChange 处理以下逻辑：
// 1. 是否需要根据 revert 回退或 abort 已存在的 runner
// 2. 是否开启新的 runner
func (p *PoSter) handleHeadChange(ctx context.Context, revert *types.TipSet, advance *types.TipSet, dinfos map[abi.ActorID]map[abi.ChainEpoch]*dline.Info) {
	// 对于当前高度来说，最少会处于1个活跃区间内，最多会处于2个活跃区间内 [next.Challenge, prev.Close)
	// 启动一个新 runner 的最基本条件为：当前高度所属的活跃区间存在没有与之对应的 runner 的情况
	currHeight := advance.Height()

	var revH *abi.ChainEpoch
	revHStr := "nil"
	if revert != nil {
		h := revert.Height()
		revH = &h
		revHStr = strconv.FormatUint(uint64(h), 10)
	}

	hcLog := log.With("rev", revHStr, "adv", advance.Height())

	pcfgs := map[abi.ActorID]*modules.MinerPoStConfig{}

	// cleanup
	for mid := range p.schedulers {
		scheds := p.schedulers[mid]
		mhcLog := hcLog.With("miner", mid)

		var pcfg *modules.MinerPoStConfig
		mcfg, err := p.cfg.MinerConfig(mid)
		if err != nil {
			mhcLog.Warnf("get miner config: %s", err)
		} else {
			pcfg = &mcfg.PoSt
			pcfgs[mid] = pcfg
		}

		for open := range scheds {
			sched := scheds[open]

			if dls, dlsOk := dinfos[mid]; dlsOk {
				// 尝试开启的 deadline 已经存在
				if _, dlOk := dls[open]; dlOk {
					delete(dls, open)
				}
			}

			// 中断此 runner
			if sched.shouldAbort(revH, currHeight) {
				mhcLog.Debugw("abort and cleanup deadline runner", "open", open)
				sched.runner.abort()
				delete(scheds, open)
				continue
			}

			// post config 由于某种原因缺失时，无法触发 start 或 submit
			if pcfg != nil && sched.isActive(currHeight) {
				if sched.shouldStart(pcfg, currHeight) {
					sched.runner.start(pcfg, advance)
				}

				if sched.couldSubmit(pcfg, currHeight) {
					sched.runner.submit(pcfg, advance)
				}
			}

		}
	}

	for mid := range dinfos {
		mdLog := hcLog.With("miner", mid)

		maddr, err := address.NewIDAddress(uint64(mid))
		if err != nil {
			mdLog.Warnf("invalid miner actor id: %s", err)
			continue
		}

		pcfg, ok := pcfgs[mid]
		if !ok {
			mcfg, err := p.cfg.MinerConfig(mid)
			if err != nil {
				mdLog.Warnf("get miner config: %s", err)
				continue
			}

			pcfg = &mcfg.PoSt
		}

		minfo, err := p.deps.minerAPI.GetInfo(ctx, mid)
		if err != nil {
			mdLog.Errorf("get miner info: %w", err)
			continue
		}

		dls := dinfos[mid]
		for open := range dls {
			dl := dls[open]

			sched := newScheduler(
				dl,
				p.runnerConstructor(
					ctx,
					p.deps,
					mid,
					maddr,
					minfo.WindowPoStProofType,
					dl,
				),
			)

			if _, ok := p.schedulers[mid]; !ok {
				p.schedulers[mid] = map[abi.ChainEpoch]*scheduler{}
			}

			mdLog.Debugw("init deadline runner", "open", dl.Open)
			p.schedulers[mid][dl.Open] = sched
			if sched.isActive(currHeight) && sched.shouldStart(pcfg, currHeight) {
				sched.runner.start(pcfg, advance)
			}
		}
	}
}
