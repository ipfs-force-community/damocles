package policy

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
)

const InteractivePoRepConfidence = 6

const MinSectorExpiration = miner.MinSectorExpiration

var GetMinSectorExpiration = policy.GetMinSectorExpiration
var GetMaxSectorExpirationExtension = policy.GetMaxSectorExpirationExtension
var GetMaxProveCommitDuration = policy.GetMaxProveCommitDuration

func GetPreCommitChallengeDelay() abi.ChainEpoch {
	// TODO: remove the guard code here
	if NetParams.Network.PreCommitChallengeDelay > 0 {
		return NetParams.Network.PreCommitChallengeDelay
	}

	return policy.GetPreCommitChallengeDelay()
}
