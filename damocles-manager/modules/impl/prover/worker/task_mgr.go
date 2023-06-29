package worker

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

type TaskState string

const (
	TaskReadyToRun TaskState = "ready2run"
	TaskRunning    TaskState = "running"
	TaskFinished   TaskState = "finished"
)

type Task struct {
	ID          string
	Input       stage.WindowPoSt
	Output      *stage.WindowPoStOutput
	tryNum      uint32
	ErrorReason string
	WorkerName  string
	StartedAt   uint64
	HeartbeatAt uint64
	FinishedAt  uint64
	CreatedAt   uint64
	UpdatedAt   uint64
}

type AllocatedTask struct {
	ID    string
	Input stage.WindowPoSt
}

type TaskManager interface {
	All(ctx context.Context, state TaskState, limit uint32, filter func(*Task) bool) ([]*Task, error)
	ListByTaskIDs(ctx context.Context, state TaskState, taskIDs ...string) ([]*Task, error)
	Create(ctx context.Context, input stage.WindowPoSt) (*Task, error)
	AllocateTasks(ctx context.Context, n uint32, workName string) (allocatedTasks []AllocatedTask, err error)
	Heartbeat(ctx context.Context, taskID []string, workerName string) error
	Finish(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error
	MakeTasksDie(ctx context.Context, shouldDeadDur time.Duration, limit uint32) error
	CleanupExpiredTasks(ctx context.Context, taskLifetime time.Duration, limit uint32) error
	RetryFailedTasks(ctx context.Context, maxTry, limit uint32) error
}

func genTaskID(rawInput []byte) string {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, xxhash.Sum64(rawInput))
	return base64.URLEncoding.EncodeToString(b)
}
