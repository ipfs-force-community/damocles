package poster

import (
	"sync/atomic"
	"testing"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v12/miner"
	"github.com/filecoin-project/go-state-types/network"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

type mockExecutor struct {
	handleFaults func(ts *types.TipSet)
	prepare      func() (*PrepareResult, error)
	generatePoSt func(*PrepareResult) ([]miner.SubmitWindowedPoStParams, error)
	submitPoSts  func(ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) error
	// record
	handleFaultsCalled int
	prepareCalled      int
	generatePoStCalled int
	submitPoStsCalled  int
}

var _ postExecutor = (*mockExecutor)(nil)

// GeneratePoSt implements postExecutor.
func (m *mockExecutor) GeneratePoSt(res *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
	m.generatePoStCalled++
	defer func() { m.generatePoSt = nil }()
	return m.generatePoSt(res)
}

// HandleFaults implements postExecutor.
func (m *mockExecutor) HandleFaults(ts *types.TipSet) {
	m.handleFaultsCalled++
	m.handleFaults(ts)
	m.handleFaults = nil
}

// Prepare implements postExecutor.
func (m *mockExecutor) Prepare() (*PrepareResult, error) {
	m.prepareCalled++
	defer func() { m.prepare = nil }()
	return m.prepare()
}

// SubmitPoSts implements postExecutor.
func (m *mockExecutor) SubmitPoSts(ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) error {
	m.submitPoStsCalled++
	defer func() { m.submitPoSts = nil }()
	return m.submitPoSts(ts, proofs)
}

type mockRunner struct {
	started  uint32
	submited uint32
	aborted  uint32
}

func (m *mockRunner) start(_ *modules.MinerPoStConfig, _ *types.TipSet) {
	atomic.AddUint32(&m.started, 1)
}

func (m *mockRunner) submit(_ *modules.MinerPoStConfig, _ *types.TipSet) {
	atomic.AddUint32(&m.submited, 1)
}

func (m *mockRunner) abort() { atomic.AddUint32(&m.aborted, 1) }

func generatePartitions(count int) []chain.Partition {
	parts := make([]chain.Partition, count)
	for i := range parts {
		empty := bitfield.New()
		for ii := 0; ii < i; ii++ {
			empty.Set(uint64(ii))
		}

		parts[i].AllSectors = empty
	}

	return parts
}

func partitionCounts(t *testing.T, batches [][]chain.Partition) map[int]struct{} {
	counts := map[int]struct{}{}
	for bi := range batches {
		for pi := range batches[bi] {
			count, err := batches[bi][pi].AllSectors.Count()
			require.NoErrorf(t, err, "get count for batches[%d][%d]", bi, pi)
			counts[int(count)] = struct{}{}
		}
	}

	return counts
}

func TestBatchPartitions(t *testing.T) {
	runner := &postRunner{}
	runner.proofType = abi.RegisteredPoStProof_StackedDrgWindow32GiBV1

	pcfg := modules.DefaultMinerPoStConfig(false)
	runner.cfg = &pcfg

	nv := network.Version16
	partitionsPerMsg, err := specpolicy.GetMaxPoStPartitions(nv, runner.proofType)
	require.NoError(t, err, "get partitions per msg")

	declMax, err := specpolicy.GetDeclarationsMax(nv)
	require.NoError(t, err, "get declaration max")

	if partitionsPerMsg > declMax {
		partitionsPerMsg = declMax
	}

	// default
	{
		partitions := make([]chain.Partition, partitionsPerMsg)
		batches, err := runner.batchPartitions(partitions, nv)
		require.NoError(t, err, "batch with default")
		require.Len(t, batches, 1, "only 1 batch allowed")
		require.Len(t, batches[0], partitionsPerMsg, "1st batch should contain all partitions")
	}

	cases := []struct {
		max    int
		counts []int
	}{
		{
			max:    1,
			counts: []int{1, 1, 1, 1, 1},
		},
		{
			max:    2,
			counts: []int{2, 2, 1},
		},
		{
			max:    3,
			counts: []int{3, 2},
		},
	}

	partCount := 5
	partitions := generatePartitions(partCount)
	for ci := range cases {
		c := cases[ci]
		require.LessOrEqual(t, c.max, partitionsPerMsg, "smaller MaxPartitionsPerPoStMessage")

		runner.cfg.MaxPartitionsPerPoStMessage = uint64(c.max)
		batches, err := runner.batchPartitions(partitions, nv)
		require.NoErrorf(t, err, "batch partitions for max=%d", c.max)

		require.Lenf(t, batches, len(c.counts), "ensure batches count for max=%d", c.max)
		for i := range c.counts {
			require.Lenf(t, batches[i], c.counts[i], "ensure #%d batch count for max=%d", i, c.max)
		}

		counts := partitionCounts(t, batches)
		require.Len(t, counts, partCount, "ensure all partitions are picked")
	}
}
