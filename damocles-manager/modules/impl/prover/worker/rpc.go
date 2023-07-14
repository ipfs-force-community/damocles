package worker

import (
	"context"
	"fmt"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

func NewWdPoStAPIImpl(taskMgr core.WorkerWdPoStTaskManager, config *Config) core.WorkerWdPoStAPI {
	return &WdPoStAPIImpl{
		taskMgr: taskMgr,
		config:  config,
	}
}

type WdPoStAPIImpl struct {
	taskMgr core.WorkerWdPoStTaskManager
	config  *Config
}

func (api WdPoStAPIImpl) WdPoStHeartbeatTasks(ctx context.Context, runningTaskIDs []string, workerName string) (core.Meta, error) {
	return nil, api.taskMgr.Heartbeat(ctx, runningTaskIDs, workerName)
}

func (api WdPoStAPIImpl) WdPoStAllocateTasks(ctx context.Context, spec core.AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []*core.WdPoStAllocatedTask, err error) {
	return api.taskMgr.AllocateTasks(ctx, spec, num, workerName)
}

func (api WdPoStAPIImpl) WdPoStFinishTask(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) (core.Meta, error) {
	return nil, api.taskMgr.Finish(ctx, taskID, output, errorReason)
}

func (api WdPoStAPIImpl) WdPoStResetTask(ctx context.Context, taskID string) (core.Meta, error) {
	// TODO(0x5459): return a friendlier error if taskID not exists
	return nil, api.taskMgr.Reset(ctx, taskID)
}

func (api WdPoStAPIImpl) WdPoStRemoveTask(ctx context.Context, taskID string) (core.Meta, error) {
	// TODO(0x5459): return a friendlier error if taskID not exists
	return nil, api.taskMgr.Remove(ctx, taskID)
}

func (api WdPoStAPIImpl) WdPoStAllTasks(ctx context.Context) ([]*core.WdPoStTask, error) {
	return api.taskMgr.All(ctx, func(_ *core.WdPoStTask) bool { return true })
}

func NewUnavailableWdPoStAPIImpl(taskMgr core.WorkerWdPoStTaskManager) core.WorkerWdPoStAPI {
	return &UnavailableWdPoStAPIImpl{}
}

// TODO(0x5459): UnavailableWdPoStAPIImpl should be automatically generated
type UnavailableWdPoStAPIImpl struct{}

func (UnavailableWdPoStAPIImpl) WdPoStHeartbeatTasks(ctx context.Context, runningTaskIDs []string, workerName string) (core.Meta, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStAllocateTasks(ctx context.Context, spec core.AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []*core.WdPoStAllocatedTask, err error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStFinishTask(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) (core.Meta, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStResetTask(ctx context.Context, taskID string) (core.Meta, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStRemoveTask(ctx context.Context, taskID string) (core.Meta, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStAllTasks(ctx context.Context) ([]*core.WdPoStTask, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}
