package worker

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil"
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

func TestAllocateJobs(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")
	m := NewKVJobManager(*kvstore.NewKVExt(kv))

	_, err := m.Create(ctx, 1, []uint64{0}, core.WdPoStInput{
		Sectors:   []core.WdPoStSectorInfo{},
		MinerID:   0,
		ProofType: "",
		Seed:      [32]byte{},
	})

	require.NoError(t, err)

	var count int = 0
	go func() {
		jobs, err := m.AllocateJobs(ctx, core.AllocateWdPoStJobSpec{
			AllowedMiners:     []abi.ActorID{},
			AllowedProofTypes: []string{},
		}, 1, "test2")
		require.NoError(t, err)
		count += len(jobs)
	}()
	jobs, err := m.AllocateJobs(ctx, core.AllocateWdPoStJobSpec{
		AllowedMiners:     []abi.ActorID{},
		AllowedProofTypes: []string{},
	}, 1, "test1")
	require.NoError(t, err)
	count += len(jobs)

	require.Equal(t, 1, count)
}
