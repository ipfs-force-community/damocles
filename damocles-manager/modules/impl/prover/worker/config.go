package worker

import "time"

type Config struct {
	RetryFailedTasksInterval       time.Duration
	TaskMaxTry                     uint32
	HeartbeatTimeout               time.Duration
	CleanupExpiredTasksJobInterval time.Duration
	TaskLifetime                   time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		RetryFailedTasksInterval:       10 * time.Second,
		TaskMaxTry:                     2,
		HeartbeatTimeout:               15 * time.Second,
		CleanupExpiredTasksJobInterval: 30 * time.Minute,
		TaskLifetime:                   25 * time.Hour,
	}
}
