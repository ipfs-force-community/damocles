package posterv2

import (
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

type BatchRunnerState int

const (
	BatchRunnerStateNotStarted BatchRunnerState = iota
	BatchRunnerStateBatched
	BatchRunnerStateComplected
)

type BatchRunner struct {
	state BatchRunnerState
}

type RunnerState int

const (
	RunnerStateNotStarted RunnerState = iota
	RunnerStateGenerating
	RunnerStateGenerated
	RunnerStateSubmiting
	RunnerStateSubmitted
)

type runner struct {
	deps *postDeps

	state      RunnerState
	partitions []chain.Partition
	proofs     proofResult
}

type proofResult struct {
	proofs []miner.SubmitWindowedPoStParams
	err    error
}

func (r *runner) handleHeadChange() {

}

func (pr *runner) generatePoSt(baseLog *logging.ZapLogger) {
	tsk := pr.startCtx.ts.Key()
	glog := baseLog.With("stage", "gen-post")

	rand, err := pr.deps.rand.GetWindowPoStChanlleengeRand(pr.ctx, tsk, pr.dinfo.Challenge, pr.mid)
	if err != nil {
		glog.Errorf("getting challenge rand: %v", err)
		return
	}

	partitions, err := pr.deps.chain.StateMinerPartitions(pr.ctx, pr.maddr, pr.dinfo.Index, tsk)
	if err != nil {
		glog.Errorf("getting partitions: %v", err)
		return
	}

	nv, err := pr.deps.chain.StateNetworkVersion(pr.ctx, tsk)
	if err != nil {
		glog.Errorf("getting network version: %v", err)
		return
	}

	// Split partitions into batches, so as not to exceed the number of sectors
	// allowed in a single message
	partitionBatches, err := pr.batchPartitions(partitions, nv)
	if err != nil {
		glog.Errorf("split partitions into batches: %v", err)
		return
	}

	batchPartitionStartIdx := 0
	for batchIdx := range partitionBatches {
		batch := partitionBatches[batchIdx]

		if pr.startCtx.pcfg.Parallel {
			go pr.generatePoStForPartitionBatch(glog, rand, batchIdx, batch, batchPartitionStartIdx)
		} else {
			pr.generatePoStForPartitionBatch(glog, rand, batchIdx, batch, batchPartitionStartIdx)
		}

		batchPartitionStartIdx += len(batch)
	}

	return
}
