package policy

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
)

const InteractivePoRepConfidence = 6

const MinSectorExpiration = miner.MinSectorExpiration

var GetMinSectorExpiration = policy.GetMinSectorExpiration
var GetMaxSectorExpirationExtension = policy.GetMaxSectorExpirationExtension
var GetDefaultSectorExpirationExtension = policy.GetDefaultSectorExpirationExtension
var GetMaxProveCommitDuration = policy.GetMaxProveCommitDuration

func GetPreCommitChallengeDelay() abi.ChainEpoch {
	// TODO: remove the guard code here
	if NetParams.PreCommitChallengeDelay > 0 {
		return NetParams.PreCommitChallengeDelay
	}

	return policy.GetPreCommitChallengeDelay()
}
