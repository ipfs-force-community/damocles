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

func NewKVTaskManager(kv kvstore.KVExt) core.WorkerWdPoStTaskManager {
	return &kvTaskManager{
		kv: kv,
	}
}

type kvTaskManager struct {
	kv kvstore.KVExt
}

// TODO(0x5459): Consider putting `txn` into context?
func (tm *kvTaskManager) filter(ctx context.Context, txn kvstore.TxnExt, state core.WdPoStTaskState, limit uint32, f func(*core.WdPoStTask) bool) (tasks []*core.WdPoStTask, err error) {
	var it kvstore.Iter
	it, err = txn.Scan([]byte(makeWdPoStPrefix(state)))
	if err != nil {
		return
	}
	defer it.Close()
	for it.Next() && len(tasks) < int(limit) {
		var task core.WdPoStTask
		if err = it.View(ctx, kvstore.LoadJSON(&task)); err != nil {
			return
		}
		if f(&task) {
			tasks = append(tasks, &task)
		}
	}
	return
}

func (tm *kvTaskManager) All(ctx context.Context, filter func(*core.WdPoStTask) bool) (tasks []*core.WdPoStTask, err error) {
	tasks = make([]*core.WdPoStTask, 0)
	err = tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, state := range []core.WdPoStTaskState{core.WdPoStTaskReadyToRun, core.WdPoStTaskRunning, core.WdPoStTaskFinished} {
			ts, err := tm.filter(ctx, txn, state, math.MaxUint32, filter)
			if err != nil {
				return err
			}
			tasks = append(tasks, ts...)
		}
		return err
	})
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt > tasks[j].CreatedAt
	})
	return
}

func (tm *kvTaskManager) ListByTaskIDs(ctx context.Context, state core.WdPoStTaskState, taskIDs ...string) ([]*core.WdPoStTask, error) {
	tasks := make([]*core.WdPoStTask, 0, len(taskIDs))
	err := tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, taskID := range taskIDs {
			var task core.WdPoStTask
			err := txn.Peek(kvstore.Key(makeWdPoStKey(state, taskID)), kvstore.LoadJSON(&task))
			if errors.Is(err, kvstore.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			tasks = append(tasks, &task)
		}
		return nil
	})
	return tasks, err
}

func (tm *kvTaskManager) Create(ctx context.Context, deadlineIdx uint64, input core.WdPoStInput) (*core.WdPoStTask, error) {
	var (
		taskID string
		task   *core.WdPoStTask
	)
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return err
		}
		taskID = GenTaskID(rawInput)
		// check if task exists
		_, err = txn.PeekAny(
			kvstore.LoadJSON(task),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskReadyToRun, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskRunning, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskFinished, taskID)),
		)
		if err == nil {
			// return if it is exists
			return nil
		}
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return err
		}

		now := time.Now().Unix()
		task = &core.WdPoStTask{
			ID:          taskID,
			State:       string(core.WdPoStTaskReadyToRun),
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
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskReadyToRun, taskID)), task)
	})

	if err == nil {
		log.Infof("wdPoSt task created: %s", taskID)
	}
	return task, err
}

func (tm *kvTaskManager) AllocateTasks(ctx context.Context, spec core.AllocateWdPoStTaskSpec, n uint32, workerName string) (allocatedTasks []*core.WdPoStAllocatedTask, err error) {
	var readyToRun []*core.WdPoStTask
	allocatedTasks = make([]*core.WdPoStAllocatedTask, 0)
	err = tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		readyToRun, err = tm.filter(ctx, txn, core.WdPoStTaskReadyToRun, n, func(t *core.WdPoStTask) bool {
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
		for _, task := range readyToRun {
			// Moving ready to run tasks to running tasks
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStTaskReadyToRun, task.ID))); err != nil {
				return err
			}
			task.State = string(core.WdPoStTaskRunning)
			task.TryNum++
			task.StartedAt = now
			task.WorkerName = workerName
			task.HeartbeatAt = now
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskRunning, task.ID)), task); err != nil {
				return err
			}
			allocatedTasks = append(allocatedTasks, &core.WdPoStAllocatedTask{
				ID:    task.ID,
				Input: task.Input,
			})
		}
		return nil
	})

	if err == nil {
		for _, task := range readyToRun {
			log.Infof("allocated wdPoSt task: %s; try_num: %d", task.ID, task.TryNum)
		}
	}
	return
}

func (tm *kvTaskManager) Heartbeat(ctx context.Context, taskIDs []string, workerName string) error {
	now := uint64(time.Now().Unix())
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, taskID := range taskIDs {
			var task core.WdPoStTask
			if err := txn.Peek([]byte(makeWdPoStKey(core.WdPoStTaskRunning, taskID)), kvstore.LoadJSON(&task)); err != nil {
				return err
			}
			if task.StartedAt == 0 {
				task.StartedAt = now
			}
			task.HeartbeatAt = now
			task.WorkerName = workerName
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskRunning, taskID)), &task); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		log.With("worker_name", workerName).Debug("wdPoSt tasks heartbeat", taskIDs)
	}
	return err
}

func (tm *kvTaskManager) Finish(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error {
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		runningKey := []byte(makeWdPoStKey(core.WdPoStTaskRunning, taskID))
		var task core.WdPoStTask
		if err := txn.Peek(runningKey, kvstore.LoadJSON(&task)); err != nil {
			return err
		}
		if err := txn.Del(runningKey); err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		task.State = string(core.WdPoStTaskFinished)
		task.Output = output
		task.ErrorReason = errorReason
		task.FinishedAt = now
		task.UpdatedAt = now
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskFinished, taskID)), &task)
	})

	if err == nil {
		if len(errorReason) == 0 {
			log.Infof("wdPoSt task succeeded: %s", taskID)
		} else {
			log.Warnf("wdPoSt task failed: %s; error_reason: %s", taskID, errorReason)
		}
	}
	return err
}

func (tm *kvTaskManager) MakeTasksDie(ctx context.Context, heartbeatTimeout time.Duration, limit uint32) error {
	var shouldDead []*core.WdPoStTask
	shouldDeadTime := time.Now().Add(-heartbeatTimeout)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldDead, err = tm.filter(ctx, txn, core.WdPoStTaskRunning, limit, func(t *core.WdPoStTask) bool {
			return t.HeartbeatAt > 0 && time.Unix(int64(t.HeartbeatAt), 0).Before(shouldDeadTime)
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, task := range shouldDead {
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStTaskRunning, task.ID))); err != nil {
				return err
			}
			task.State = string(core.WdPoStTaskFinished)
			task.FinishedAt = now
			task.Output = nil
			task.ErrorReason = "heartbeat timeout"
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskFinished, task.ID)), task); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, task := range shouldDead {
			log.Infof("make wdPoSt task die: %s; heartbeat_at: %s", task.ID, time.Unix(int64(task.HeartbeatAt), 0).Format(time.RFC3339))
		}
	}

	return err
}

func (tm *kvTaskManager) CleanupExpiredTasks(ctx context.Context, taskLifetime time.Duration, limit uint32) error {
	var shouldClean []*core.WdPoStTask
	shouldCleanTime := time.Now().Add(-taskLifetime)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldClean, err = tm.filter(ctx, txn, core.WdPoStTaskFinished, limit, func(t *core.WdPoStTask) bool {
			return time.Unix(int64(t.CreatedAt), 0).Before(shouldCleanTime)
		})
		if err != nil {
			return err
		}
		for _, task := range shouldClean {
			if err := txn.Del([]byte(makeWdPoStKey(core.WdPoStTaskFinished, task.ID))); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, task := range shouldClean {
			log.Infof("cleanup expired wdPoSt task: %s; created_at: %s", task.ID, time.Unix(int64(task.CreatedAt), 0).Format(time.RFC3339))
		}
	}
	return err
}

func (tm *kvTaskManager) RetryFailedTasks(ctx context.Context, maxTry, limit uint32) error {
	var shouldRetry []*core.WdPoStTask
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldRetry, err = tm.filter(ctx, txn, core.WdPoStTaskFinished, limit, func(t *core.WdPoStTask) bool {
			return len(t.ErrorReason) != 0 && t.TryNum < maxTry
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, task := range shouldRetry {
			task.ErrorReason = ""
			task.State = string(core.WdPoStTaskReadyToRun)
			task.Output = nil
			task.StartedAt = 0
			task.FinishedAt = 0
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskReadyToRun, task.ID)), task); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, task := range shouldRetry {
			log.Debugf("retry wdPoSt task: %s; try_num: %d, error_reason: %s", task.ID, task.TryNum, task.ErrorReason)
		}
	}

	return err
}

func (tm *kvTaskManager) Reset(ctx context.Context, taskID string) error {
	var task core.WdPoStTask
	now := uint64(time.Now().Unix())

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		key, err := txn.PeekAny(
			kvstore.LoadJSON(&task),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskReadyToRun, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskRunning, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskFinished, taskID)),
		)
		if err != nil {
			return fmt.Errorf("load task from db: %w. taskID: %s", err, taskID)
		}

		task.State = string(core.WdPoStTaskReadyToRun)
		task.CreatedAt = now
		task.StartedAt = 0
		task.TryNum = 0
		task.Output = nil
		task.ErrorReason = ""
		task.FinishedAt = 0
		task.HeartbeatAt = 0
		task.WorkerName = ""
		task.UpdatedAt = now

		if err := txn.Del(key); err != nil {
			return err
		}
		return txn.PutJson([]byte(makeWdPoStKey(core.WdPoStTaskReadyToRun, taskID)), &task)
	})

	if err == nil {
		log.Infof("task is reset: %s", taskID)
	}

	return err
}

func (tm *kvTaskManager) Remove(ctx context.Context, taskID string) error {
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		key, err := txn.PeekAny(
			kvstore.NilF,
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskReadyToRun, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskRunning, taskID)),
			kvstore.Key(makeWdPoStKey(core.WdPoStTaskFinished, taskID)),
		)
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load task from db: %w. taskID: %s", err, taskID)
		}
		return txn.Del(key)
	})

	if err == nil {
		log.Infof("task removed: %s", taskID)
	}

	return err
}

const (
	prefixTaskIDdelimiter = ":"
)

func makeWdPoStPrefix(state core.WdPoStTaskState) string {
	return string(state)
}

func makeWdPoStKey(state core.WdPoStTaskState, taskID string) string {
	return fmt.Sprintf("%s%s%s", makeWdPoStPrefix(state), prefixTaskIDdelimiter, taskID)
}

//lint:ignore U1000 Ignore unused function
func splitKey(key string) (state core.WdPoStTaskState, taskID string) {
	x := strings.SplitN(key, prefixTaskIDdelimiter, 2)
	return core.WdPoStTaskState(x[0]), x[1]
}
