package worker

import (
	"context"
	"fmt"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/extproc/stage"
)

func NewWdPoStAPIImpl(taskMgr core.WorkerWdPoStTaskManager) core.WorkerWdPoStAPI {
	return &WdPoStAPIImpl{
		taskMgr: taskMgr,
	}
}

type WdPoStAPIImpl struct {
	taskMgr core.WorkerWdPoStTaskManager
}

func (api WdPoStAPIImpl) WdPoStHeartbeatTask(ctx context.Context, runningTaskIDs []string, workerName string) error {
	return api.taskMgr.Heartbeat(ctx, runningTaskIDs, workerName)
}

func (api WdPoStAPIImpl) WdPoStAllocateTasks(ctx context.Context, spec core.AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []*core.WdPoStAllocatedTask, err error) {
	return api.taskMgr.AllocateTasks(ctx, spec, num, workerName)
}

func (api WdPoStAPIImpl) WdPoStFinishTask(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error {
	return api.taskMgr.Finish(ctx, taskID, output, errorReason)
}

func (api WdPoStAPIImpl) WdPoStResetTask(ctx context.Context, taskID string) error {
	return api.taskMgr.Reset(ctx, taskID)
}

func (api WdPoStAPIImpl) WdPoStAllTasks(ctx context.Context) ([]*core.WdPoStTask, error) {
	return api.taskMgr.All(ctx, func(_ *core.WdPoStTask) bool { return true })
}

// TODO(0x5459): UnavailableWdPoStAPIImpl should be automatically generated
type UnavailableWdPoStAPIImpl struct{}

func (UnavailableWdPoStAPIImpl) WdPoStHeartbeatTask(ctx context.Context, runningTaskIDs []string, workerName string) error {
	return fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStAllocateTasks(ctx context.Context, spec core.AllocateWdPoStTaskSpec, num uint32, workerName string) (allocatedTasks []core.WdPoStAllocatedTask, err error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStFinishTask(ctx context.Context, taskID string, output *stage.WindowPoStOutput, errorReason string) error {
	return fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStResetTask(ctx context.Context, taskID string) error {
	return fmt.Errorf("WdPoStAPI unavailable")
}

func (UnavailableWdPoStAPIImpl) WdPoStAllTasks(ctx context.Context) ([]*core.WdPoStTask, error) {
	return nil, fmt.Errorf("WdPoStAPI unavailable")
}
