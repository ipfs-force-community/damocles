package worker

import "time"

type Config struct {
	RetryFailedJobsInterval     time.Duration
	JobMaxTry                   uint32
	HeartbeatTimeout            time.Duration
	CleanupExpiredJobsInterval time.Duration
	JobLifetime                 time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		RetryFailedJobsInterval:     10 * time.Second,
		JobMaxTry:                   2,
		HeartbeatTimeout:            15 * time.Second,
		CleanupExpiredJobsInterval: 30 * time.Minute,
		JobLifetime:                 25 * time.Hour,
	}
}
