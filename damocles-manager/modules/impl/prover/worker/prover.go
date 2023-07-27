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

var _ core.Prover = (*WorkerProver)(nil)

func GenJobID(rawInput []byte) string {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, xxhash.Sum64(rawInput))
	return base58.Encode(b)
}

type R struct {
	output *stage.WindowPoStOutput
	err    string
}

type WorkerProver struct {
	jobMgr        core.WorkerWdPoStJobManager
	sectorTracker core.SectorTracker
	localProver   core.Prover

	inflightJobs     map[string][]chan<- R
	inflightJobsLock *sync.Mutex
	config           *Config
}

func NewProver(jobMgr core.WorkerWdPoStJobManager, sectorTracker core.SectorTracker, config *Config) *WorkerProver {
	return &WorkerProver{
		jobMgr:           jobMgr,
		sectorTracker:    sectorTracker,
		localProver:      prover.NewProdProver(sectorTracker),
		inflightJobs:     make(map[string][]chan<- R),
		inflightJobsLock: &sync.Mutex{},
		config:           config,
	}
}

func (p *WorkerProver) Start(ctx context.Context) {
	go p.runNotifyJobDone(ctx)
	go p.runRetryFailedJobs(ctx)
	go p.runCleanupExpiredJobs(ctx)
}

func (p *WorkerProver) runNotifyJobDone(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("stop notifyJobDone")
			return
		case <-ticker.C:
			p.inflightJobsLock.Lock()
			inflightJobIDs := make([]string, 0, len(p.inflightJobs))
			for jobID := range p.inflightJobs {
				inflightJobIDs = append(inflightJobIDs, jobID)
			}
			p.inflightJobsLock.Unlock()

			finishedJobs, err := p.jobMgr.ListByJobIDs(ctx, core.WdPoStJobFinished, inflightJobIDs...)
			if err != nil {
				log.Errorf("failed to list jobs: %s", err)
			}

			p.inflightJobsLock.Lock()
			for _, job := range finishedJobs {
				chs, ok := p.inflightJobs[job.ID]
				if !ok {
					continue
				}
				if !job.Finished(p.config.JobMaxTry) {
					continue
				}
				delete(p.inflightJobs, job.ID)

				for _, ch := range chs {
					ch <- R{
						output: job.Output,
						err:    job.ErrorReason,
					}
					close(ch)
				}
			}
			p.inflightJobsLock.Unlock()
		}
	}
}

func (p *WorkerProver) runRetryFailedJobs(ctx context.Context) {
	ticker := time.NewTicker(p.config.RetryFailedJobsInterval)
	defer ticker.Stop()
	for {
		if err := p.jobMgr.MakeJobsDie(ctx, p.config.HeartbeatTimeout, 128); err != nil {
			log.Errorf("failed to make jobs die: %s", err)
		}
		if err := p.jobMgr.RetryFailedJobs(ctx, p.config.JobMaxTry, 128); err != nil {
			log.Errorf("failed to retry failed jobs: %s", err)
		}
		select {
		case <-ctx.Done():
			log.Info("stop retryFailedJobs")
			return
		case <-ticker.C:
			continue
		}
	}
}

func (p *WorkerProver) runCleanupExpiredJobs(ctx context.Context) {
	ticker := time.NewTicker(p.config.CleanupExpiredJobsInterval)
	for {
		if err := p.jobMgr.CleanupExpiredJobs(ctx, p.config.JobLifetime, 128); err != nil {
			log.Errorf("failed to cleanup expired jobs: %s", err)
		}
		select {
		case <-ctx.Done():
			log.Info("stop cleanupExpiredJobs")
			return
		case <-ticker.C:
			continue
		}
	}
}

func (p *WorkerProver) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return p.localProver.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *WorkerProver) GenerateWindowPoSt(ctx context.Context, deadlineIdx uint64, minerID abi.ActorID, proofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, randomness abi.PoStRandomness) (proof []builtin.PoStProof, skipped []abi.SectorID, err error) {

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
		ProofType: stage.ProofType2String(proofType),
		Sectors:   sis,
	}
	copy(input.Seed[:], randomness[:])

	job, err := p.jobMgr.Create(ctx, deadlineIdx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("create wdPoSt job: %w", err)
	}

	ch := make(chan R, 1)

	p.inflightJobsLock.Lock()
	p.inflightJobs[job.ID] = append(p.inflightJobs[job.ID], ch)
	p.inflightJobsLock.Unlock()

	var result R
	select {
	case <-ctx.Done():
		err = fmt.Errorf("failed to generate window post before context cancellation: %w", ctx.Err())
		return
	case res, ok := <-ch:
		if !ok {
			return nil, nil, fmt.Errorf("wdPoSt result channel was closed unexpectedly")
		}
		result = res
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

func (p *WorkerProver) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, proofType abi.RegisteredPoStProof, sectors []builtin.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return p.localProver.GenerateWinningPoSt(ctx, minerID, proofType, sectors, randomness)
}

func (p *WorkerProver) GeneratePoStFallbackSectorChallenges(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, sectorIds []abi.SectorNumber) (*core.FallbackChallenges, error) {
	return p.localProver.GeneratePoStFallbackSectorChallenges(ctx, proofType, minerID, randomness, sectorIds)
}

func (p *WorkerProver) GenerateSingleVanillaProof(ctx context.Context, replica core.FFIPrivateSectorInfo, challenges []uint64) ([]byte, error) {
	return p.localProver.GenerateSingleVanillaProof(ctx, replica, challenges)
}

func (p *WorkerProver) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]core.PoStProof, error) {
	return p.localProver.GenerateWinningPoStWithVanilla(ctx, proofType, minerID, randomness, proofs)
}
