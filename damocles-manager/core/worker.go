package core

import (
	"fmt"
	"time"
)

const DefaultWorkerListenPort = 17890

type SealingThreadState struct {
	Ty   string  `json:"type"`
	Secs *uint64 `json:"secs"`
}

func (s SealingThreadState) String() string {
	if s.Secs == nil {
		return s.Ty
	}
	return fmt.Sprintf("%s(%s)", s.Ty, time.Duration(*s.Secs)*time.Second)
}

type WorkerThreadInfo struct {
	Index       int                `json:"index"`
	Location    string             `json:"location"`
	Plan        string             `json:"plan"`
	JobID       *string            `json:"job_id"`
	ThreadState SealingThreadState `json:"thread_state"`
	JobState    string             `json:"job_state"`
	JobStage    *string            `json:"job_stage"`
	LastError   *string            `json:"last_error"`
}

type WorkerInfoSummary struct {
	Threads uint
	Empty   uint
	Paused  uint
	Running uint
	Waiting uint
	Errors  uint
}

type WorkerPingInfo struct {
	Info     WorkerInfo
	LastPing int64
}

type WorkerInfo struct {
	Name    string
	Dest    string
	Version string
	Summary WorkerInfoSummary
}
