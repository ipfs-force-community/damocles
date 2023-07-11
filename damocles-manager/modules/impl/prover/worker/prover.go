package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/mr-tron/base58/base58"
)

var log = logging.New("worker prover")

func GenTaskID(rawInput []byte) string {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, xxhash.Sum64(rawInput))
	return base58.Encode(b)
}

type workerProver struct {
	taskMgr       core.WorkerWdPoStTaskManager
	sectorTracker core.SectorTracker
	localProver   core.Prover

	inflightTasks map[string][]chan<- struct {
		output *stage.WindowPoStOutput
		err    string
	}
	inflightTasksLock *sync.Mutex

	retryFailedTasksInterval time.Duration
	taskMaxTry               uint32
	heartbeatTimeout         time.Duration

	cleanupExpiredTasksJobInterval time.Duration
	taskLifetime                   time.Duration
}

func NewProver(taskMgr core.WorkerWdPoStTaskManager, sectorTracker core.SectorTracker) core.Prover {
	return &workerProver{
		taskMgr:       taskMgr,
		sectorTracker: sectorTracker,
		localProver:   prover.NewProdProver(sectorTracker),
		inflightTasks: make(map[string][]chan<- struct {
			output *stage.WindowPoStOutput
			err    string
		}),
		inflightTasksLock: &sync.Mutex{},

		// TODO(0x5459): make them configurable
		retryFailedTasksInterval:       10 * time.Second,
		taskMaxTry:                     2,
		heartbeatTimeout:               15 * time.Second,
		cleanupExpiredTasksJobInterval: 30 * time.Minute,
		taskLifetime:                   25 * time.Hour,
	}
}

func (p *workerProver) StartJob(ctx context.Context) {
	go p.runNotifyTaskDoneJob(ctx)
	go p.runRetryFailedTasksJob(ctx)
	go p.runCleanupExpiredTasksJob(ctx)
}

func (p *workerProver) runNotifyTaskDoneJob(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.inflightTasksLock.Lock()
			inflightTaskIDs := make([]string, 0, len(p.inflightTasks))
			for taskID := range p.inflightTasks {
				inflightTaskIDs = append(inflightTaskIDs, taskID)
			}
			p.inflightTasksLock.Unlock()

			finishedTasks, err := p.taskMgr.ListByTaskIDs(ctx, core.WdPoStTaskFinished, inflightTaskIDs...)
			if err != nil {
				log.Errorf("failed to list tasks: %s", err)
			}

			p.inflightTasksLock.Lock()
			for _, task := range finishedTasks {
				chs, ok := p.inflightTasks[task.ID]
				if !ok {
					continue
				}
				if !task.Finished(p.taskMaxTry) {
					continue
				}
				for _, ch := range chs {
					ch <- struct {
						output *stage.WindowPoStOutput
						err    string
					}{
						output: task.Output,
						err:    task.ErrorReason,
					}
				}
			}
			p.inflightTasksLock.Unlock()
		}
	}
}

func (p *workerProver) runRetryFailedTasksJob(ctx context.Context) {
	ticker := time.NewTicker(p.retryFailedTasksInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.taskMgr.MakeTasksDie(ctx, p.heartbeatTimeout, 128); err != nil {
				log.Errorf("failed to make tasks die: %s", err)
			}
			if err := p.taskMgr.RetryFailedTasks(ctx, p.taskMaxTry, 128); err != nil {
				log.Errorf("failed to retry failed tasks: %s", err)
			}
		}
	}
}

func (p *workerProver) runCleanupExpiredTasksJob(ctx context.Context) {
	ticker := time.NewTicker(p.cleanupExpiredTasksJobInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.taskMgr.CleanupExpiredTasks(ctx, p.taskLifetime, 128); err != nil {
				log.Errorf("failed to cleanup expired tasks: %s", err)
			}
		}
	}
}

func (p *workerProver) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return p.localProver.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *workerProver) GenerateWindowPoSt(ctx context.Context, deadlineIdx uint64, minerID abi.ActorID, proofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, randomness abi.PoStRandomness) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {

	sis := make([]core.WdPoStSectorInfo, len(sectors))
	for i, s := range sectors {
		privInfo, err := p.sectorTracker.SinglePubToPrivateInfo(ctx, minerID, s, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("construct private info for %d: %w", s.SectorNumber, err)
		}
		commR, err := util.CID2ReplicaCommitment(s.SealedCID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid sealed cid %s for sector %d of miner %d: %w", s.SealedCID, s.SectorNumber, minerID, err)
		}

		sis[i] = core.WdPoStSectorInfo{
			SectorID: s.SectorNumber,
			CommR:    commR,
			Upgrade:  s.SectorKey != nil,
			Accesses: privInfo.Accesses,
		}
	}

	input := core.WdPoStInput{
		MinerID:   minerID,
		ProofType: proofType,
		Sectors:   sis,
	}
	copy(input.Seed[:], randomness[:])

	task, err := p.taskMgr.Create(ctx, deadlineIdx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("create wdPoSt task: %w", err)
	}

	ch := make(chan struct {
		output *stage.WindowPoStOutput
		err    string
	}, 1)

	p.inflightTasksLock.Lock()
	p.inflightTasks[task.ID] = append(p.inflightTasks[task.ID], ch)
	p.inflightTasksLock.Unlock()

	result, ok := <-ch

	if !ok {
		return nil, nil, fmt.Errorf("wdPoSt result channel was closed unexpectedly")
	}
	if result.err != "" {
		return nil, nil, fmt.Errorf("error from worker: %s", result.err)
	}

	if faultCount := len(result.output.Faults); faultCount != 0 {
		faults := make([]abi.SectorID, faultCount)
		for fi := range result.output.Faults {
			faults[fi] = abi.SectorID{
				Miner:  minerID,
				Number: result.output.Faults[fi],
			}
		}

		return nil, faults, fmt.Errorf("got %d fault sectors", faultCount)
	}

	proofs := make([]builtin.PoStProof, len(result.output.Proofs))
	for pi := range result.output.Proofs {
		proofs[pi] = builtin.PoStProof{
			PoStProof:  proofType,
			ProofBytes: result.output.Proofs[pi],
		}
	}

	return proofs, nil, nil
}

func (p *workerProver) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, proofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return p.localProver.GenerateWinningPoSt(ctx, minerID, proofType, sectors, randomness)
}

func (p *workerProver) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return p.localProver.GeneratePoStFallbackSectorChallenges(ctx, proofType, minerID, randomness, sectorIds)
}

func (p *workerProver) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return p.localProver.GenerateSingleVanillaProof(ctx, replica, challenges)
}

func (p *workerProver) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	return p.localProver.GenerateWinningPoStWithVanilla(ctx, proofType, minerID, randomness, proofs)
}
