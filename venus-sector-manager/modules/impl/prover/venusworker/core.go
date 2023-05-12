package venusworker

import "context"

type Task struct {
	ID          string
	InBody      []byte
	RetryCount  uint32
	ErrorReason *string
	OutBody     []byte
	WorkerName  *string
	StartedAt   *uint64
	HeartbeatAt *uint64
	FinishedAt  *uint64
	CreatedAt   uint64
	UpdatedAt   uint64
}

type TaskManager interface {
	All(ctx context.Context, filter func(*Task) bool) ([]*Task, error)
	Create(ctx context.Context, taskID string, inBody []byte) error
	Finish(ctx context.Context, taskID string, finishedAt uint64, outBody []byte, errorReason *string) error
}
