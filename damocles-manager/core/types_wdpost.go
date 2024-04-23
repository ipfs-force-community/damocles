package core

import (
	"context"
	"encoding/json"
	"fmt"
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
	Partitions  []uint64
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

type AllWdPoStJob struct {
	Jobs   []WdPoStJobBrief
	MaxTry uint32
}

type WdPoStJobBrief struct {
	*WdPoStJob
	Sectors uint32
	Faults  uint32
}

func (j *WdPoStJobBrief) DisplayState() string {
	switch WdPoStJobState(j.State) {
	case WdPoStJobReadyToRun:
		return "ReadyToRun"
	case WdPoStJobRunning:
		return "Running"
	case WdPoStJobFinished:
		if j.Succeed() {
			if j.Faults == 0 {
				return "Succeed"
			}
			return fmt.Sprintf("Faults(%d)", j.Faults)
		}
		return "Failed"
	}
	return j.State
}

func (j *WdPoStJobBrief) MarshalJSON() ([]byte, error) {
	j.WdPoStJob.Input = WdPoStInput{
		MinerID: j.WdPoStJob.Input.MinerID,
	}
	j.WdPoStJob.Output = &stage.WindowPoStOutput{}

	return json.Marshal(struct {
		*WdPoStJob
		Sectors uint32
		Faults  uint32
	}{
		WdPoStJob: j.WdPoStJob,
		Sectors:   j.Sectors,
		Faults:    j.Faults,
	})
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
	Create(ctx context.Context, deadlineIdx uint64, partitions []uint64, input WdPoStInput) (*WdPoStJob, error)
	AllocateJobs(
		ctx context.Context,
		spec AllocateWdPoStJobSpec,
		num uint32,
		workerName string,
	) (allocatedJobs []*WdPoStAllocatedJob, err error)
	Heartbeat(ctx context.Context, jobIDs []string, workerName string) error
	Finish(ctx context.Context, jobID string, output *stage.WindowPoStOutput, errorReason string) error
	MakeJobsDie(ctx context.Context, shouldDeadDur time.Duration, limit uint32) error
	CleanupExpiredJobs(ctx context.Context, jobLifetime time.Duration, limit uint32) error
	RetryFailedJobs(ctx context.Context, maxTry, limit uint32) error
	Reset(ctx context.Context, jobID string) error
	Remove(ctx context.Context, jobID string) error
}
