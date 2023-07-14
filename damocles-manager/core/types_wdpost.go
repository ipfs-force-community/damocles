package core

import (
	"context"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

type WdPoStSectorInfo struct {
	SectorID abi.SectorNumber `json:"SectorId"`
	CommR    [32]byte
	Upgrade  bool // is upgrade sector
	Accesses SectorAccessStores
}

type WdPoStInput struct {
	Sectors   []WdPoStSectorInfo
	MinerID   abi.ActorID `json:"MinerId"`
	ProofType string
	Seed      [32]byte
}

type WdPoStTaskState string

const (
	WdPoStTaskReadyToRun WdPoStTaskState = "ready2run"
	WdPoStTaskRunning    WdPoStTaskState = "running"
	WdPoStTaskFinished   WdPoStTaskState = "finished"
)

type WdPoStTask struct {
	ID          string `json:"Id"`
	State       string
	DeadlineIdx uint64
	Input       WdPoStInput
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
	ID    string `json:"Id"`
	Input WdPoStInput
}

type AllocateWdPoStTaskSpec struct {
	AllowedMiners     []abi.ActorID
	AllowedProofTypes []abi.RegisteredPoStProof
}

type WorkerWdPoStTaskManager interface {
	All(ctx context.Context, filter func(*WdPoStTask) bool) ([]*WdPoStTask, error)
	ListByTaskIDs(ctx context.Context, state WdPoStTaskState, taskIDs ...string) ([]*WdPoStTask, error)
	Create(ctx context.Context, deadlineIdx uint64, input WdPoStInput) (*WdPoStTask, error)
	AllocateTasks(ctx context.Context, spec AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []*WdPoStAllocatedTask, err error)
	Heartbeat(ctx context.Context, taskIDs []string, workerName string) error
	Finish(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error
	MakeTasksDie(ctx context.Context, shouldDeadDur time.Duration, limit uint32) error
	CleanupExpiredTasks(ctx context.Context, taskLifetime time.Duration, limit uint32) error
	RetryFailedTasks(ctx context.Context, maxTry, limit uint32) error
	Reset(ctx context.Context, taskID string) error
	Remove(ctx context.Context, taskID string) error
}
