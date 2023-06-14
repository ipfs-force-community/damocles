package poster

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
)

func newScheduler(dl *dline.Info, runner PoStRunner) *scheduler {
	return &scheduler{
		dl:     dl,
		runner: runner,
	}
}

type scheduler struct {
	dl     *dline.Info
	runner PoStRunner
}

// 在 [Challenge, Close) 区间内
func (s *scheduler) isActive(height abi.ChainEpoch) bool {
	return deadlineIsActive(s.dl, height)
}

// 在 [Challenge + ChallengeConfidence, Close) 区间内
func (s *scheduler) shouldStart(pcfg *modules.MinerPoStConfig, height abi.ChainEpoch) bool {
	if !pcfg.Enabled {
		return false
	}

	challengeConfidence := pcfg.ChallengeConfidence
	if challengeConfidence == 0 {
		challengeConfidence = DefaultChallengeConfidence
	}

	// Check if the chain is above the Challenge height for the post window
	return height >= s.dl.Challenge+abi.ChainEpoch(challengeConfidence)
}

func (s *scheduler) couldSubmit(pcfg *modules.MinerPoStConfig, height abi.ChainEpoch) bool {
	submitConfidence := pcfg.SubmitConfidence
	if submitConfidence == 0 {
		submitConfidence = DefaultSubmitConfidence
	}

	return height >= s.dl.Open+abi.ChainEpoch(submitConfidence)
}

// 回退至 Challenge 之前，或已到达 Close 之后
func (s *scheduler) shouldAbort(revert *abi.ChainEpoch, advance abi.ChainEpoch) bool {
	if revert != nil && *revert < s.dl.Challenge {
		return true
	}

	return advance >= s.dl.Close
}
