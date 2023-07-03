package worker

import (
	"testing"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/stretchr/testify/require"
)

func TestSplitKey(t *testing.T) {
	for _, taskID := range []string{"normal123", "with-", "-", "-with", "wi-th", "with:xxx", ":xxx", ":"} {
		for _, state := range []core.WdPoStTaskState{core.WdPoStTaskReadyToRun, core.WdPoStTaskRunning, core.WdPoStTaskFinished} {
			actualState, actualTaskID := splitKey(makeWdPoStKey(state, taskID))
			require.Equalf(t, state, actualState, "test state for \"state: `%s`; taskID: `%s`\"", state, taskID)
			require.Equalf(t, taskID, actualTaskID, "test taskID for \"state: `%s`; taskID: `%s`\"", state, taskID)
		}
	}
}
