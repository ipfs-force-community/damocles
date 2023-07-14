package core

const DefaultWorkerListenPort = 17890

type WorkerThreadInfo struct {
	Index         int     `json:"index"`
	Location      string  `json:"location"`
	Plan          string  `json:"plan"`
	JobID         *string `json:"job_id"`
	Paused        bool    `json:"paused"`
	PausedElapsed *uint64 `json:"paused_elapsed"`
	State         string  `json:"state"`
	LastError     *string `json:"last_error"`
}

type WorkerInfoSummary struct {
	Threads uint
	Empty   uint
	Paused  uint
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
