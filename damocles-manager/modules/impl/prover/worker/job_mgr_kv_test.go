package worker

import (
	"testing"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/stretchr/testify/require"
)

func TestSplitKey(t *testing.T) {
	for _, jobID := range []string{"normal123", "with-", "-", "-with", "wi-th", "with:xxx", ":xxx", ":"} {
		for _, state := range []core.WdPoStJobState{core.WdPoStJobReadyToRun, core.WdPoStJobRunning, core.WdPoStJobFinished} {
			actualState, actualJobID := splitKey(makeWdPoStKey(state, jobID))
			require.Equalf(t, state, actualState, "test state for \"state: `%s`; jobID: `%s`\"", state, jobID)
			require.Equalf(t, jobID, actualJobID, "test jobID for \"state: `%s`; jobID: `%s`\"", state, jobID)
		}
	}
}
