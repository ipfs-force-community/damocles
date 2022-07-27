package poster

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
)

type scheduler struct {
	dl     *dline.Info
	runner PoStRunner
}

// 在 [Challenge, Close) 区间内
func (s *scheduler) isActive(height abi.ChainEpoch) bool {
	return height >= s.dl.Challenge && height < s.dl.Close
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

	if height < s.dl.Open+abi.ChainEpoch(submitConfidence) {
		return false
	}

	return true
}

// 回退至 Open 之前，或已到达 Close 之后
func (s *scheduler) shouldAbort(revert *types.TipSet, advance *types.TipSet) bool {
	if revert != nil && revert.Height() < s.dl.Open {
		return true
	}

	if advance.Height() >= s.dl.Close {
		return true
	}

	return false
}
