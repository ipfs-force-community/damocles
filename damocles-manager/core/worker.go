package core

import (
	"fmt"
	"time"
)

const DefaultWorkerListenPort = 17890

type SealingThreadState struct {
	State   string  `json:"state"`
	Elapsed *uint64 `json:"elapsed"`
	Proc    *string `json:"proc"`
}

func (s SealingThreadState) String() string {
	if s.Elapsed == nil {
		return s.State
	}

	var proc string
	if s.Proc == nil {
		proc = ""
	} else {
		proc = *s.Proc
	}

	return fmt.Sprintf("%s(%s) %s", s.State, time.Duration(*s.Elapsed)*time.Second, proc)
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
