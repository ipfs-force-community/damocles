package policy

import (
	"github.com/filecoin-project/go-state-types/abi"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
)

const (
	EpochDurationSeconds           = builtin5.EpochDurationSeconds
	ChainFinality                  = miner5.ChainFinality
	SealRandomnessLookback         = ChainFinality
	MaxPreCommitRandomnessLookback = builtin5.EpochsInDay + SealRandomnessLookback
)

func GetPreCommitChallengeDelay() abi.ChainEpoch {
	return miner5.PreCommitChallengeDelay
}
