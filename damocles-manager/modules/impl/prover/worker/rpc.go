package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

func NewWdPoStAPIImpl(jobMgr core.WorkerWdPoStJobManager, config *Config) core.WorkerWdPoStAPI {
	return &WdPoStAPIImpl{
		jobMgr: jobMgr,
		config: config,
	}
}

type WdPoStAPIImpl struct {
	jobMgr core.WorkerWdPoStJobManager
	config *Config
}

func (api WdPoStAPIImpl) WdPoStHeartbeatJobs(ctx context.Context, runningJobIDs []string, workerName string) (core.Meta, error) {
	return nil, api.jobMgr.Heartbeat(ctx, runningJobIDs, workerName)
}

func (api WdPoStAPIImpl) WdPoStAllocateJobs(ctx context.Context, spec core.AllocateWdPoStJobSpec, num uint32, workerName string) (allocatedJobs []*core.WdPoStAllocatedJob, err error) {
	return api.jobMgr.AllocateJobs(ctx, spec, num, workerName)
}

func (api WdPoStAPIImpl) WdPoStFinishJob(ctx context.Context, jobID string, output *stage.WindowPoStOutput, errorReason string) (core.Meta, error) {
	return nil, api.jobMgr.Finish(ctx, jobID, output, errorReason)
}

func (api WdPoStAPIImpl) WdPoStResetJob(ctx context.Context, jobID string) (core.Meta, error) {
	err := api.jobMgr.Reset(ctx, jobID)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return nil, fmt.Errorf("job '%s' does not exist", jobID)
	}
	return nil, err
}

func (api WdPoStAPIImpl) WdPoStRemoveJob(ctx context.Context, jobID string) (core.Meta, error) {
	err := api.jobMgr.Remove(ctx, jobID)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return nil, fmt.Errorf("job '%s' does not exist", jobID)
	}
	return nil, err
}

func (api WdPoStAPIImpl) WdPoStAllJobs(ctx context.Context) ([]*core.WdPoStJob, error) {
	return api.jobMgr.All(ctx, func(_ *core.WdPoStJob) bool { return true })
}
