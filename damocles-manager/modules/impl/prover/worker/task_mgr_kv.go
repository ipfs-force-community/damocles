package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

func NewKVTaskStore(kv kvstore.KVExt) TaskManager {
	return &kvTaskManager{
		kv: kv,
	}
}

type kvTaskManager struct {
	kv kvstore.KVExt
}

// TODO(0x5459): Consider putting `txn` into context?
func (tm *kvTaskManager) filter(ctx context.Context, txn kvstore.TxnExt, state TaskState, limit uint32, f func(*Task) bool) (tasks []*Task, err error) {
	var it kvstore.Iter
	it, err = txn.Scan([]byte(makeWdPoStPrefix(state)))
	if err != nil {
		return
	}
	defer it.Close()
	for it.Next() && len(tasks) <= int(limit) {
		var task Task
		if err = it.View(ctx, kvstore.LoadJSON(&task)); err != nil {
			return
		}
		if f(&task) {
			tasks = append(tasks, &task)
		}
	}
	return
}

func (tm *kvTaskManager) All(ctx context.Context, state TaskState, limit uint32, filter func(*Task) bool) (tasks []*Task, err error) {
	err = tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		tasks, err = tm.filter(ctx, txn, state, limit, filter)
		return err
	})
	return
}

func (tm *kvTaskManager) ListByTaskIDs(ctx context.Context, state TaskState, taskIDs ...string) ([]*Task, error) {
	tasks := make([]*Task, 0, len(taskIDs))
	err := tm.kv.ViewMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, taskID := range taskIDs {
			var task Task
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

func (tm *kvTaskManager) Create(ctx context.Context, input stage.WindowPoSt) (*Task, error) {
	var (
		taskID string
		task   *Task
	)
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		rawInput, err := json.Marshal(input)
		if err != nil {
			return err
		}
		taskID = GenTaskID(rawInput)
		// check if task exists
		err = txn.PeekAny(
			kvstore.LoadJSON(task),
			kvstore.Key(makeWdPoStKey(TaskReadyToRun, taskID)),
			kvstore.Key(makeWdPoStKey(TaskRunning, taskID)),
			kvstore.Key(makeWdPoStKey(TaskFinished, taskID)),
		)
		if err == nil {
			// return if it is exists
			return nil
		}
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return err
		}

		now := time.Now().Unix()
		task = &Task{
			ID:          taskID,
			Input:       input,
			Output:      nil,
			tryNum:      0,
			ErrorReason: "",
			WorkerName:  "",
			StartedAt:   0,
			HeartbeatAt: 0,
			FinishedAt:  0,
			CreatedAt:   uint64(now),
			UpdatedAt:   uint64(now),
		}
		return txn.PutJson([]byte(makeWdPoStKey(TaskReadyToRun, taskID)), task)
	})

	if err == nil {
		log.Infof("wdPoSt task created: %s", taskID)
	}
	return task, err
}

func (tm *kvTaskManager) AllocateTasks(ctx context.Context, n uint32, workName string) (allocatedTasks []AllocatedTask, err error) {
	var readyToRun []*Task
	err = tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		readyToRun, err = tm.filter(ctx, txn, TaskReadyToRun, n, func(t *Task) bool { return true })
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, task := range readyToRun {
			task.tryNum++
			task.StartedAt = now
			task.WorkerName = workName
			task.HeartbeatAt = now
			task.UpdatedAt = now
			// Moving ready to run tasks to running tasks
			if err := txn.Del([]byte(makeWdPoStKey(TaskReadyToRun, task.ID))); err != nil {
				return err
			}
			if err := txn.PutJson([]byte(makeWdPoStKey(TaskRunning, task.ID)), task); err != nil {
				return err
			}
			allocatedTasks = append(allocatedTasks, AllocatedTask{
				ID:    task.ID,
				Input: task.Input,
			})
		}
		return nil
	})

	if err == nil {
		for _, task := range readyToRun {
			log.Infof("allocated wdPoSt task: %s; try_num: %d", task.ID, task.tryNum)
		}
	}
	return
}

func (tm *kvTaskManager) Heartbeat(ctx context.Context, taskIDs []string, workerName string) error {
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		for _, taskID := range taskIDs {
			var task Task
			if err := txn.Peek([]byte(makeWdPoStKey(TaskRunning, taskID)), kvstore.LoadJSON(&task)); err != nil {
				return err
			}
			now := uint64(time.Now().Unix())
			task.HeartbeatAt = now
			task.WorkerName = workerName
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(TaskRunning, taskID)), &task); err != nil {
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
		runningKey := []byte(makeWdPoStKey(TaskRunning, taskID))
		var task Task
		if err := txn.Peek(runningKey, kvstore.LoadJSON(&task)); err != nil {
			return err
		}
		if err := txn.Del(runningKey); err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		task.Output = output
		task.ErrorReason = errorReason
		task.FinishedAt = now
		task.UpdatedAt = now
		return txn.PutJson([]byte(makeWdPoStKey(TaskFinished, taskID)), &task)
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
	var shouldDead []*Task
	shouldDeadTime := time.Now().Add(-heartbeatTimeout)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldDead, err = tm.filter(ctx, txn, TaskRunning, limit, func(t *Task) bool {
			return t.HeartbeatAt > 0 && time.Unix(int64(t.HeartbeatAt), 0).Before(shouldDeadTime)
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, task := range shouldDead {
			if err := txn.Del([]byte(makeWdPoStKey(TaskRunning, task.ID))); err != nil {
				return err
			}
			task.FinishedAt = now
			task.Output = nil
			task.ErrorReason = "heartbeat timeout"
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(TaskFinished, task.ID)), task); err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

func (tm *kvTaskManager) CleanupExpiredTasks(ctx context.Context, taskLifetime time.Duration, limit uint32) error {
	var shouldClean []*Task
	shouldCleanTime := time.Now().Add(-taskLifetime)

	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldClean, err = tm.filter(ctx, txn, TaskFinished, limit, func(t *Task) bool {
			return time.Unix(int64(t.CreatedAt), 0).Before(shouldCleanTime)
		})
		if err != nil {
			return err
		}
		for _, task := range shouldClean {
			if err := txn.Del([]byte(makeWdPoStKey(TaskFinished, task.ID))); err != nil {
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
	var shouldRetry []*Task
	err := tm.kv.UpdateMustNoConflict(ctx, func(txn kvstore.TxnExt) error {
		var err error
		shouldRetry, err = tm.filter(ctx, txn, TaskFinished, limit, func(t *Task) bool {
			return len(t.ErrorReason) != 0 && t.tryNum > maxTry
		})
		if err != nil {
			return err
		}
		now := uint64(time.Now().Unix())
		for _, task := range shouldRetry {
			task.ErrorReason = ""
			task.Output = nil
			task.StartedAt = 0
			task.FinishedAt = 0
			task.UpdatedAt = now
			if err := txn.PutJson([]byte(makeWdPoStKey(TaskFinished, task.ID)), task); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		for _, task := range shouldRetry {
			log.Debugf("retry wdPoSt task: %d; try_num: %d, error_reason: %s", task.ID, task.tryNum)
		}
	}

	return err
}

func makeWdPoStPrefix(state TaskState) string {
	return fmt.Sprintf("wdpost-%s-", state)
}

func makeWdPoStKey(state TaskState, taskID string) string {
	return fmt.Sprintf("%s%s", makeWdPoStPrefix(state), taskID)
}
