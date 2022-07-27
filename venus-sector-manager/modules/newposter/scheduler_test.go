package poster

import (
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randomDeadline() *dline.Info {
	periodStart := miner.WPoStProvingPeriod + abi.ChainEpoch(rand.Intn(int(10*miner.WPoStProvingPeriod)))
	index := uint64(rand.Intn(int(miner.WPoStPeriodDeadlines)))

	return dline.NewInfo(
		periodStart,
		index,
		0,
		miner.WPoStPeriodDeadlines,
		miner.WPoStProvingPeriod,
		miner.WPoStChallengeWindow,
		miner.WPoStChallengeLookback,
		miner.FaultDeclarationCutoff,
	)
}

func TestScheduler(t *testing.T) {
	t.Run("isAcive", func(t *testing.T) {
		for i := 0; i < 128; i++ {
			dl := randomDeadline()
			sched := newScheduler(dl, mockRunner{})

			require.Falsef(t, sched.isActive(dl.Challenge-1), "%#v: before challenge", dl)
			require.Truef(t, sched.isActive(dl.Challenge), "%#v: challenge", dl)
			require.Truef(t, sched.isActive(dl.Open), "%#v: open", dl)
			require.Falsef(t, sched.isActive(dl.Close), "%#v: close", dl)
			require.Falsef(t, sched.isActive(dl.Close+1), "%#v: after close", dl)
		}
	})

	t.Run("shouldStart", func(t *testing.T) {
		for i := 0; i < 128; i++ {
			dl := randomDeadline()
			sched := newScheduler(dl, mockRunner{})
			pcfg := modules.DefaultMinerPoStConfig(false)

			start := dl.Challenge
			confidence := abi.ChainEpoch(DefaultChallengeConfidence)

			// 重置可能影响的字段，测试默认逻辑
			pcfg.Enabled = true
			pcfg.ChallengeConfidence = 0

			require.Falsef(t, sched.shouldStart(&pcfg, start), "%#v: challenge", dl)
			require.Falsef(t, sched.shouldStart(&pcfg, start+confidence-1), "%#v: before default confidence", dl)
			require.Truef(t, sched.shouldStart(&pcfg, start+confidence), "%#v: default confidence", dl)

			// half default challenge confidence
			half := confidence / 2
			require.Falsef(t, sched.shouldStart(&pcfg, start+half), "%#v: half with default confidence", dl)

			pcfg.ChallengeConfidence = uint64(half)
			require.Falsef(t, sched.shouldStart(&pcfg, start+half-1), "%#v: before half with customized confidence", dl)
			require.Truef(t, sched.shouldStart(&pcfg, start+half), "%#v: half with customized confidence", dl)

			// disabled
			pcfg.Enabled = false
			require.Falsef(t, sched.shouldStart(&pcfg, start+half), "%#v: half with customized confidence, disabled", dl)

		}
	})

	t.Run("couldSubmit", func(t *testing.T) {
		for i := 0; i < 128; i++ {
			dl := randomDeadline()
			sched := newScheduler(dl, mockRunner{})
			pcfg := modules.DefaultMinerPoStConfig(false)

			start := dl.Open
			confidence := abi.ChainEpoch(DefaultSubmitConfidence)

			// 重置可能影响的字段，测试默认逻辑
			pcfg.Enabled = true
			pcfg.SubmitConfidence = 0

			require.Falsef(t, sched.couldSubmit(&pcfg, start), "%#v: open", dl)
			require.Falsef(t, sched.couldSubmit(&pcfg, start+confidence-1), "%#v: before default confidence", dl)
			require.Truef(t, sched.couldSubmit(&pcfg, start+confidence), "%#v: default confidence", dl)

			// half default challenge confidence
			half := confidence / 2
			require.Falsef(t, sched.couldSubmit(&pcfg, start+half), "%#v: half with default confidence", dl)

			pcfg.SubmitConfidence = uint64(half)
			require.Falsef(t, sched.couldSubmit(&pcfg, start+half-1), "%#v: before half with customized confidence", dl)
			require.Truef(t, sched.couldSubmit(&pcfg, start+half), "%#v: half with customized confidence", dl)

			// disabled
			pcfg.Enabled = false
			require.Truef(t, sched.couldSubmit(&pcfg, start+half), "%#v: half with customized confidence, disabled", dl)
		}
	})

	t.Run("shouldAbort", func(t *testing.T) {
		for i := 0; i < 128; i++ {
			dl := randomDeadline()
			sched := newScheduler(dl, mockRunner{})

			require.Falsef(t, sched.shouldAbort(nil, dl.Open), "%#v: advanced to open", dl)
			require.Falsef(t, sched.shouldAbort(nil, dl.Last()), "%#v: advanced to last", dl)
			require.Truef(t, sched.shouldAbort(nil, dl.Close), "%#v: advanced to close", dl)

			require.Falsef(t, sched.shouldAbort(&dl.Open, dl.Open), "%#v: revert to open", dl)

			before := dl.Open - 1
			require.True(t, sched.shouldAbort(&before, dl.Open), "%#v: revert to before", dl)
		}
	})

}
