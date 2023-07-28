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

type WdPoStJobState string

const (
	WdPoStJobReadyToRun WdPoStJobState = "ready2run"
	WdPoStJobRunning    WdPoStJobState = "running"
	WdPoStJobFinished   WdPoStJobState = "finished"
)

type WdPoStJob struct {
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

func (t *WdPoStJob) Finished(maxTry uint32) bool {
	if t.State != string(WdPoStJobFinished) {
		return false
	}

	if t.ErrorReason != "" && t.TryNum < maxTry {
		return false
	}

	return true
}

func (t *WdPoStJob) Succeed() bool {
	if t.State != string(WdPoStJobFinished) {
		return false
	}
	return t.ErrorReason == ""
}

func (t *WdPoStJob) DisplayState() string {
	switch WdPoStJobState(t.State) {
	case WdPoStJobReadyToRun:
		return "ReadyToRun"
	case WdPoStJobRunning:
		return "Running"
	case WdPoStJobFinished:
		if t.Succeed() {
			return "Succeed"
		} else {
			return "Failed"
		}
	}
	return t.State
}

type WdPoStAllocatedJob struct {
	ID    string `json:"Id"`
	Input WdPoStInput
}

type AllocateWdPoStJobSpec struct {
	AllowedMiners     []abi.ActorID
	AllowedProofTypes []string
}

type WorkerWdPoStJobManager interface {
	All(ctx context.Context, filter func(*WdPoStJob) bool) ([]*WdPoStJob, error)
	ListByJobIDs(ctx context.Context, jobIDs ...string) ([]*WdPoStJob, error)
	Create(ctx context.Context, deadlineIdx uint64, input WdPoStInput) (*WdPoStJob, error)
	AllocateJobs(ctx context.Context, spec AllocateWdPoStJobSpec, num uint32, workerName string) (allocatedJobs []*WdPoStAllocatedJob, err error)
	Heartbeat(ctx context.Context, jobIDs []string, workerName string) error
	Finish(ctx context.Context, jobID string, output *stage.WindowPoStOutput, errorReason string) error
	MakeJobsDie(ctx context.Context, shouldDeadDur time.Duration, limit uint32) error
	CleanupExpiredJobs(ctx context.Context, jobLifetime time.Duration, limit uint32) error
	RetryFailedJobs(ctx context.Context, maxTry, limit uint32) error
	Reset(ctx context.Context, jobID string) error
	Remove(ctx context.Context, jobID string) error
}
