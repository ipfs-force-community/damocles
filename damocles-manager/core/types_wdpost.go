package core

import (
	"context"
	"time"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

type WdPoStTaskState string

const (
	WdPoStTaskReadyToRun WdPoStTaskState = "ready2run"
	WdPoStTaskRunning    WdPoStTaskState = "running"
	WdPoStTaskFinished   WdPoStTaskState = "finished"
)

type WdPoStTask struct {
	ID          string
	Input       stage.WindowPoSt
	Output      *stage.WindowPoStOutput
	TryNum      uint32
	ErrorReason string
	WorkerName  string
	StartedAt   uint64
	HeartbeatAt uint64
	FinishedAt  uint64
	CreatedAt   uint64
	UpdatedAt   uint64
}

func (t *WdPoStTask) Finished(maxTry uint32) bool {
	if t.FinishedAt == 0 {
		return false
	}

	if t.ErrorReason != "" && t.TryNum < maxTry {
		return false
	}

	return true
}

type WdPoStAllocatedTask struct {
	ID    string
	Input stage.WindowPoSt
}

type WorkerWdPoStTaskManager interface {
	All(ctx context.Context, filter func(*WdPoStTask) bool) ([]*WdPoStTask, error)
	ListByTaskIDs(ctx context.Context, state WdPoStTaskState, taskIDs ...string) ([]*WdPoStTask, error)
	Create(ctx context.Context, input stage.WindowPoSt) (*WdPoStTask, error)
	AllocateTasks(ctx context.Context, num uint32, workName string) (allocatedTasks []WdPoStAllocatedTask, err error)
	Heartbeat(ctx context.Context, taskIDs []string, workerName string) error
	Finish(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error
	MakeTasksDie(ctx context.Context, shouldDeadDur time.Duration, limit uint32) error
	CleanupExpiredTasks(ctx context.Context, taskLifetime time.Duration, limit uint32) error
	RetryFailedTasks(ctx context.Context, maxTry, limit uint32) error
	Reset(ctx context.Context, taskID string) error
}
