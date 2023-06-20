package sectors

import (
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	stminer "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/stretchr/testify/require"
)

func TestDeadlineIsMutable(t *testing.T) {
	cases := []struct {
		targetDeadline         int
		currentDeadline        int
		currentEpochInDeadline abi.ChainEpoch
		isMut                  bool
		delay                  abi.ChainEpoch
	}{
		{
			targetDeadline:         10,
			currentDeadline:        9,
			currentEpochInDeadline: 0,
			isMut:                  false,
			delay:                  2 * stminer.WPoStChallengeWindow,
		},
		{
			targetDeadline:         10,
			currentDeadline:        8,
			currentEpochInDeadline: 1,
			isMut:                  true,
			delay:                  0,
		},
		{
			targetDeadline:         10,
			currentDeadline:        9,
			currentEpochInDeadline: 1,
			isMut:                  false,
			delay:                  2*stminer.WPoStChallengeWindow - 1,
		},
		{
			targetDeadline:         10,
			currentDeadline:        10,
			currentEpochInDeadline: 0,
			isMut:                  false,
			delay:                  stminer.WPoStChallengeWindow,
		},
		{
			targetDeadline:         10,
			currentDeadline:        10,
			currentEpochInDeadline: 1,
			isMut:                  false,
			delay:                  stminer.WPoStChallengeWindow - 1,
		},
		{
			targetDeadline:         10,
			currentDeadline:        11,
			currentEpochInDeadline: 0,
			isMut:                  true,
			delay:                  0,
		},
		{
			targetDeadline:         0,
			currentDeadline:        47,
			currentEpochInDeadline: 0,
			isMut:                  false,
			delay:                  2 * stminer.WPoStChallengeWindow,
		},
		{
			targetDeadline:         0,
			currentDeadline:        47,
			currentEpochInDeadline: 1,
			isMut:                  false,
			delay:                  2*stminer.WPoStChallengeWindow - 1,
		},
		{
			targetDeadline:         0,
			currentDeadline:        0,
			currentEpochInDeadline: 0,
			isMut:                  false,
			delay:                  stminer.WPoStChallengeWindow,
		},
	}
	for _, c := range cases {
		currentEpoch := abi.ChainEpoch(c.currentDeadline*int(stminer.WPoStChallengeWindow)) + c.currentEpochInDeadline
		ddl := stminer.NewDeadlineInfo(abi.ChainEpoch(0), uint64(c.targetDeadline), currentEpoch)
		isMut, delay := deadlineIsMutable(ddl.PeriodStart, ddl.Index, currentEpoch, 0)
		require.Equal(t, c.isMut, isMut, "test isMut for `%v`", c)
		require.Equal(t, c.delay, delay, "test delay for `%v`", c)
	}

}
