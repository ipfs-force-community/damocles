package poster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

const (
	DefaultSubmitConfidence    = 4
	DefaultChallengeConfidence = 10
)

var log = logging.New("poster")

type postRunnerInner interface {
	// 开启当前的证明窗口
	start(pctx postContext)
	// 提交当前证明结果
	submit(pctx postContext)
	// 终止当前证明任务
	abort(pctx postContext)
}

type postRunner struct {
	mid   abi.ActorID
	dl    *dline.Info
	inner postRunnerInner
}

// 在 [Challenge, Close) 区间内
func (r *postRunner) isActive(height abi.ChainEpoch) bool {
	return height >= r.dl.Challenge && height < r.dl.Close
}

// 在 [Challenge + ChallengeConfidence, Close) 区间内
func (r *postRunner) shouldStart(pctx postContext, height abi.ChainEpoch) bool {
	mcfg, err := pctx.cfg.MinerConfig(r.mid)
	if err != nil {
		return false
	}

	challengeConfidence := mcfg.PoSt.ChallengeConfidence
	if challengeConfidence == 0 {
		challengeConfidence = DefaultChallengeConfidence
	}

	// Check if the chain is above the Challenge height for the post window
	return height >= r.dl.Challenge+abi.ChainEpoch(challengeConfidence)
}

func (r *postRunner) couldSubmit(pctx postContext, height abi.ChainEpoch) bool {
	mcfg, err := pctx.cfg.MinerConfig(r.mid)
	if err != nil {
		return false
	}

	submitConfidence := mcfg.PoSt.SubmitConfidence
	if submitConfidence == 0 {
		submitConfidence = DefaultSubmitConfidence
	}

	if height < r.dl.Open+abi.ChainEpoch(submitConfidence) {
		return false
	}

	return true
}

// 回退至 Open 之前，或已到达 Close 之后
func (r *postRunner) shouldAbort(pctx postContext, revert *types.TipSet, advance *types.TipSet) bool {
	if revert != nil && revert.Height() < r.dl.Open {
		return true
	}

	if advance.Height() >= r.dl.Close {
		return true
	}

	return false
}

type postContext struct {
	cfg   *modules.SafeConfig
	chain chain.API
}

type PoSter struct {
	pctx postContext

	runners                map[abi.ActorID]map[abi.ChainEpoch]*postRunner
	runnerInnerConstructor func(mid abi.ActorID, maddr address.Address) postRunnerInner
}

func (p *PoSter) handleChainNotify(ctx context.Context) {

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

			ch, err := p.pctx.chain.ChainNotify(ctx)
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

			p.pctx.cfg.Lock()
			mids := make([]abi.ActorID, 0, len(p.pctx.cfg.Miners))
			for mi := range p.pctx.cfg.Miners {
				if mcfg := p.pctx.cfg.Miners[mi]; mcfg.PoSt.Enabled {
					if !mcfg.PoSt.Sender.Valid() {
						mlog.Warnw("post is enabled, but sender is invalid", "miner", mcfg.Actor)
						continue
					}

					mids = append(mids, mcfg.Actor)
				}
			}
			p.pctx.cfg.Unlock()

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

func (p *PoSter) fetchMinerProvingDeadlineInfos(ctx context.Context, mids []abi.ActorID, ts *types.TipSet) map[abi.ActorID]*dline.Info {
	count := len(mids)
	infos := make([]*dline.Info, count)
	errs := make([]error, count)

	tsk := ts.Key()

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

			dinfo, err := p.pctx.chain.StateMinerProvingDeadline(ctx, maddr, tsk)
			if err != nil {
				errs[idx] = fmt.Errorf("rpc call: %w", err)
				return
			}

			infos[idx] = dinfo
		}(i)
	}

	wg.Wait()

	res := map[abi.ActorID]*dline.Info{}
	for i := range mids {
		if err := errs[i]; err != nil {
			log.Warnf("fetch proving deadline for %d: %s", mids[i], err)
			continue
		}

		res[mids[i]] = infos[i]
	}

	return res
}

// handleHeadChange 处理以下逻辑：
// 1. 是否需要根据 revert 回退或 abort 已存在的 runner
// 2. 是否开启新的 runner
func (p *PoSter) handleHeadChange(ctx context.Context, revert *types.TipSet, advance *types.TipSet, dinfos map[abi.ActorID]*dline.Info) {
	// 对于当前高度来说，最少会处于1个活跃区间内，最多会处于2个活跃区间内 [next.Challenge, prev.Close)
	// 启动一个新 runner 的最基本条件为：当前高度所属的活跃区间存在没有与之对应的 runner 的情况
	currHeight := advance.Height()

	// cleanup
	for mid := range p.runners {
		runners := p.runners[mid]
		for open := range runners {
			// 尝试开启的 deadline 已经存在
			if dl, ok := dinfos[mid]; ok && dl.Open == open {
				delete(dinfos, mid)
			}

			runner := runners[open]
			// 中断此 runner
			if runner.shouldAbort(p.pctx, revert, advance) {
				runner.inner.abort(p.pctx)
				delete(runners, open)
				continue
			}

			if runner.isActive(currHeight) {
				if runner.shouldStart(p.pctx, currHeight) {
					runner.inner.start(p.pctx)
				}

				if runner.couldSubmit(p.pctx, currHeight) {
					runner.inner.submit(p.pctx)
				}
			}

		}
	}

	for mid := range dinfos {
		maddr, err := address.NewIDAddress(uint64(mid))
		if err != nil {
			log.Warnf("invalid miner actor id %d: %w", mid, maddr)
			continue
		}

		dl := dinfos[mid]
		prunner := &postRunner{
			mid:   mid,
			dl:    dl,
			inner: p.runnerInnerConstructor(mid, maddr),
		}

		if _, ok := p.runners[mid]; !ok {
			p.runners = map[abi.ActorID]map[abi.ChainEpoch]*postRunner{}
		}

		p.runners[mid][dl.Open] = prunner
		if prunner.isActive(currHeight) {
			prunner.inner.start(p.pctx)
		}
	}
}
