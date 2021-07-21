package policy

import (
	"github.com/filecoin-project/venus/pkg/specactors/builtin"
	"github.com/filecoin-project/venus/pkg/specactors/policy"
)

const (
	EpochDurationSeconds           = builtin.EpochDurationSeconds
	ChainFinality                  = policy.ChainFinality
	SealRandomnessLookback         = ChainFinality
	MaxPreCommitRandomnessLookback = builtin.EpochsInDay + SealRandomnessLookback
)

var GetPreCommitChallengeDelay = policy.GetPreCommitChallengeDelay
