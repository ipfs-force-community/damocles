package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"golang.org/x/exp/slices"
)

func NewKVJobManager(kv kvstore.KVExt) core.WorkerWdPoStJobManager {
	return &kvJobManager{
		kv: kv,
	}
}

type kvJobManager struct {
	kv kvstore.KVExt
}

// TODO(0x5459): Consider putting `txn` into context?
func (tm *kvJobManager) filter(ctx context.Context, txn kvstore.TxnExt, state core.WdPoStJobState, limit uint32, f func(*core.WdPoStJob) bool) (jobs []*core.WdPoStJob, err error) {
	var it kvstore.Iter
	it, err = txn.Scan([]byte(makeWdPoStPrefix(state)))
	if err != nil {
		return
	}
	defer it.Close()
	for it.Next() && len(jobs) < int(limit) {
		var job core.WdPoStJob
		if err = it.View(ctx, kvstore.LoadJSON(&job)); err != nil {
			return
		}
		if f(&job) {
			jobs = append(jobs, &job)
		}
	}
	return
}

func (tm *kvJobManager) All(ctx context.Context, filter func(*core.WdPoStJob) bool) (jobs []*core.WdPoStJob, err error) {
	jobs = make([]*core.WdPoStJob, 0)
	err = tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, state := range []core.WdPoStJobState{core.WdPoStJobReadyToRun, core.WdPoStJobRunning, core.WdPoStJobFinished} {
			ts, err := tm.filter(ctx, txn, state, math.MaxUint32, filter)
			if err != nil {
				return err
			}
			jobs = append(jobs, ts...)
		}
		return err
	})
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt > jobs[j].CreatedAt
	})
	return
}

func (tm *kvJobManager) ListByJobIDs(ctx context.Context, state core.WdPoStJobState, jobIDs ...string) ([]*core.WdPoStJob, error) {
	jobs := make([]*core.WdPoStJob, 0, len(jobIDs))
	err := tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, jobID := range jobIDs {
			var job core.WdPoStJob
			err := txn.Peek(kvstore.Key(makeWdPoStKey(state, jobID)), kvstore.LoadJSON(&job))
			if errors.Is(err, kvstore.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			jobs = append(jobs, &job)
		}
		return nil
	})
	return jobs, err
}

func (tm *kvJobManager) Create(ctx context.Context, deadlineIdx uint64, input core.WdPoStInput) (*core.WdPoStJob, error) {
	var (
		jobID string
		job   *core.WdPoStJob
	)
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return err
		}
		jobID = GenJobID(rawInput)
		// check if job exists
		_, err = txn.PeekAny(
			kvstore.LoadJSON(job),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobReadyToRun, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobRunning, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobFinished, jobID)),
		)
		if err == nil {
			// return if it is exists
			return nil
		}
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return err
		}

		now := time.Now().Unix()
		job = &core.WdPoStJob{
			ID:          jobID,
			State:       string(core.WdPoStJobReadyToRun),
			DeadlineIdx: deadlineIdx,
			Input:       input,
			Output:      nil,
			TryNum:      0,
			ErrorReason: "",
			WorkerName:  "",
			StartedAt:   0,
			HeartbeatAt: 0,
			FinishedAt:  0,
			CreatedAt:   uint64(now),
			UpdatedAt:   uint64(now),
		}
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobReadyToRun, jobID)), job)
	})

	if err == nil {
		log.Infof("wdPoSt job created: %s", jobID)
	}
	return job, err
}

func (tm *kvJobManager) AllocateJobs(ctx context.Context, spec core.AllocateWdPoStJobSpec, n uint32, workerName string) (allocatedJobs []*core.WdPoStAllocatedJob, err error) {
	var readyToRun []*core.WdPoStJob
	allocatedJobs = make([]*core.WdPoStAllocatedJob, 0)
	err = tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		readyToRun, err = tm.filter(ctx, txn, core.WdPoStJobReadyToRun, n, func(t *core.WdPoStJob) bool {
			if len(spec.AllowedMiners) > 0 && !slices.Contains(spec.AllowedMiners, t.Input.MinerID) {
				return false
			}
			if len(spec.AllowedProofTypes) > 0 && !slices.ContainsFunc(spec.AllowedProofTypes, func(allowed abi.RegisteredPoStProof) bool {
				return stage.ProofType2String(allowed) == t.Input.ProofType
			}) {
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, job := range readyToRun {
			// Moving ready to run jobs to running jobs
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStJobReadyToRun, job.ID))); err != nil {
				return err
			}
			job.State = string(core.WdPoStJobRunning)
			job.TryNum++
			job.StartedAt = now
			job.WorkerName = workerName
			job.HeartbeatAt = now
			job.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobRunning, job.ID)), job); err != nil {
				return err
			}
			allocatedJobs = append(allocatedJobs, &core.WdPoStAllocatedJob{
				ID:    job.ID,
				Input: job.Input,
			})
		}
		return nil
	})

	if err == nil {
		for _, job := range readyToRun {
			log.Infof("allocated wdPoSt job: %s; try_num: %d", job.ID, job.TryNum)
		}
	}
	return
}

func (tm *kvJobManager) Heartbeat(ctx context.Context, jobIDs []string, workerName string) error {
	now := uint64(time.Now().Unix())
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, jobID := range jobIDs {
			var job core.WdPoStJob
			if err := txn.Peek([]byte(makeWdPoStKey(core.WdPoStJobRunning, jobID)), kvstore.LoadJSON(&job)); err != nil {
				return err
			}
			if job.StartedAt == 0 {
				job.StartedAt = now
			}
			job.HeartbeatAt = now
			job.WorkerName = workerName
			job.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobRunning, jobID)), &job); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		log.With("worker_name", workerName).Debug("wdPoSt jobs heartbeat", jobIDs)
	}
	return err
}

func (tm *kvJobManager) Finish(ctx context.Context, jobID string, output *stage.WindowPoStOutput, errorReason string) error {
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		runningKey := []byte(makeWdPoStKey(core.WdPoStJobRunning, jobID))
		var job core.WdPoStJob
		if err := txn.Peek(runningKey, kvstore.LoadJSON(&job)); err != nil {
			return err
		}
		if err := txn.Del(runningKey); err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		job.State = string(core.WdPoStJobFinished)
		job.Output = output
		job.ErrorReason = errorReason
		job.FinishedAt = now
		job.UpdatedAt = now
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobFinished, jobID)), &job)
	})

	if err == nil {
		if len(errorReason) == 0 {
			log.Infof("wdPoSt job succeeded: %s", jobID)
		} else {
			log.Warnf("wdPoSt job failed: %s; error_reason: %s", jobID, errorReason)
		}
	}
	return err
}

func (tm *kvJobManager) MakeJobsDie(ctx context.Context, heartbeatTimeout time.Duration, limit uint32) error {
	var shouldDead []*core.WdPoStJob
	shouldDeadTime := time.Now().Add(-heartbeatTimeout)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldDead, err = tm.filter(ctx, txn, core.WdPoStJobRunning, limit, func(t *core.WdPoStJob) bool {
			return t.HeartbeatAt > 0 && time.Unix(int64(t.HeartbeatAt), 0).Before(shouldDeadTime)
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, job := range shouldDead {
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStJobRunning, job.ID))); err != nil {
				return err
			}
			job.State = string(core.WdPoStJobFinished)
			job.FinishedAt = now
			job.Output = nil
			job.ErrorReason = "heartbeat timeout"
			job.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobFinished, job.ID)), job); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, job := range shouldDead {
			log.Infof("make wdPoSt job die: %s; heartbeat_at: %s", job.ID, time.Unix(int64(job.HeartbeatAt), 0).Format(time.RFC3339))
		}
	}

	return err
}

func (tm *kvJobManager) CleanupExpiredJobs(ctx context.Context, jobLifetime time.Duration, limit uint32) error {
	var shouldClean []*core.WdPoStJob
	shouldCleanTime := time.Now().Add(-jobLifetime)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldClean, err = tm.filter(ctx, txn, core.WdPoStJobFinished, limit, func(t *core.WdPoStJob) bool {
			return time.Unix(int64(t.CreatedAt), 0).Before(shouldCleanTime)
		})
		if err != nil {
			return err
		}
		for _, job := range shouldClean {
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStJobFinished, job.ID))); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, job := range shouldClean {
			log.Infof("cleanup expired wdPoSt job: %s; job: %#v", job.ID, job)
		}
	}
	return err
}

func (tm *kvJobManager) RetryFailedJobs(ctx context.Context, maxTry, limit uint32) error {
	var shouldRetry []*core.WdPoStJob
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldRetry, err = tm.filter(ctx, txn, core.WdPoStJobFinished, limit, func(t *core.WdPoStJob) bool {
			return len(t.ErrorReason) != 0 && t.TryNum < maxTry
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, job := range shouldRetry {
			err := txn.Del([]byte(makeWdPoStKey(core.WdPoStJobFinished, job.ID)))
			if err != nil {
				return err
			}
			job.ErrorReason = ""
			job.State = string(core.WdPoStJobReadyToRun)
			job.Output = nil
			job.StartedAt = 0
			job.FinishedAt = 0
			job.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobReadyToRun, job.ID)), job); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, job := range shouldRetry {
			log.Debugf("retry wdPoSt job: %s; try_num: %d, error_reason: %s", job.ID, job.TryNum, job.ErrorReason)
		}
	}

	return err
}

func (tm *kvJobManager) Reset(ctx context.Context, jobID string) error {
	var job core.WdPoStJob
	now := uint64(time.Now().Unix())

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		key, err := txn.PeekAny(
			kvstore.LoadJSON(&job),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobReadyToRun, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobRunning, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobFinished, jobID)),
		)
		if err != nil {
			return fmt.Errorf("load job from db: %w. jobID: %s", err, jobID)
		}

		job.State = string(core.WdPoStJobReadyToRun)
		job.CreatedAt = now
		job.StartedAt = 0
		job.TryNum = 0
		job.Output = nil
		job.ErrorReason = ""
		job.FinishedAt = 0
		job.HeartbeatAt = 0
		job.WorkerName = ""
		job.UpdatedAt = now

		if err := txn.Del(key); err != nil {
			return err
		}
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStJobReadyToRun, jobID)), &job)
	})

	if err == nil {
		log.Infof("job is reset: %s", jobID)
	}

	return err
}

func (tm *kvJobManager) Remove(ctx context.Context, jobID string) error {
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		key, err := txn.PeekAny(
			kvstore.NilF,
			kvstore.Key(makeWdPoStKey(core.WdPoStJobReadyToRun, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobRunning, jobID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStJobFinished, jobID)),
		)
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load job from db: %w. jobID: %s", err, jobID)
		}
		return txn.Del(key)
	})

	if err == nil {
		log.Infof("job removed: %s", jobID)
	}

	return err
}

const (
	prefixJobIDdelimiter = ":"
)

func makeWdPoStPrefix(state core.WdPoStJobState) string {
	return string(state)
}

func makeWdPoStKey(state core.WdPoStJobState, jobID string) string {
	return fmt.Sprintf("%s%s%s", makeWdPoStPrefix(state), prefixJobIDdelimiter, jobID)
}

//lint:ignore U1000 Ignore unused function
func splitKey(key string) (state core.WdPoStJobState, jobID string) {
	x := strings.SplitN(key, prefixJobIDdelimiter, 2)
	return core.WdPoStJobState(x[0]), x[1]
}
