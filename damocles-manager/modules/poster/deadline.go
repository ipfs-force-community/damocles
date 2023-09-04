package poster

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
)

// a few samples of dline.Info:
// {
//     "CurrentEpoch":1143656,
//     "PeriodStart":1143609,
//     "Index":0,
//     "Open":1143609,
//     "Close":1143669,
//     "Challenge":1143589,
//     "FaultCutoff":1143539,
//     "WPoStPeriodDeadlines":48,
//     "WPoStProvingPeriod":2880,
//     "WPoStChallengeWindow":60,
//     "WPoStChallengeLookback":20,
//     "FaultDeclarationCutoff":70
// }
//
// {
//     "CurrentEpoch":1144232,
//     "PeriodStart":1143609,
//     "Index":10,
//     "Open":1144209,
//     "Close":1144269,
//     "Challenge":1144189,
//     "FaultCutoff":1144139,
//     "WPoStPeriodDeadlines":48,
//     "WPoStProvingPeriod":2880,
//     "WPoStChallengeWindow":60,
//     "WPoStChallengeLookback":20,
//     "FaultDeclarationCutoff":70
// }
//
// {
//     "CurrentEpoch":1143664,
//     "PeriodStart":1142556,
//     "Index":31,
//     "Open":1144416,
//     "Close":1144476,
//     "Challenge":1144396,
//     "FaultCutoff":1144346,
//     "WPoStPeriodDeadlines":48,
//     "WPoStProvingPeriod":2880,
//     "WPoStChallengeWindow":60,
//     "WPoStChallengeLookback":20,
//     "FaultDeclarationCutoff":70
// }

// nextDeadline gets deadline info for the subsequent deadline
func nextDeadline(currentDeadline *dline.Info, _ abi.ChainEpoch) *dline.Info {
	periodStart := currentDeadline.PeriodStart
	newDeadlineIndex := currentDeadline.Index + 1
	if newDeadlineIndex == currentDeadline.WPoStPeriodDeadlines {
		newDeadlineIndex = 0
		periodStart = periodStart + currentDeadline.WPoStProvingPeriod
	}

	return dline.NewInfo(
		periodStart,
		newDeadlineIndex,
		currentDeadline.CurrentEpoch,
		currentDeadline.WPoStPeriodDeadlines,
		currentDeadline.WPoStProvingPeriod,
		currentDeadline.WPoStChallengeWindow,
		currentDeadline.WPoStChallengeLookback,
		currentDeadline.FaultDeclarationCutoff,
	)
}

func deadlineIsActive(dl *dline.Info, epoch abi.ChainEpoch) bool {
	return epoch >= dl.Challenge && epoch < dl.Close
}
