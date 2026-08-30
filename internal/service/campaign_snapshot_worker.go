package service

import (
	"context"
	"errors"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type CampaignSnapshotWorker struct {
	snapshots *CampaignSnapshotService
}

func NewCampaignSnapshotWorker(snapshots *CampaignSnapshotService) (*CampaignSnapshotWorker, error) {
	if snapshots == nil {
		return nil, errors.New("campaign snapshot service is required")
	}
	return &CampaignSnapshotWorker{snapshots: snapshots}, nil
}

func (w *CampaignSnapshotWorker) CanProcess(taskType string) bool {
	return taskType == domain.SnapshotCampaignTaskType
}

func (w *CampaignSnapshotWorker) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	if task == nil || task.State == nil || task.State.SnapshotCampaign == nil || task.State.SnapshotCampaign.RunID == "" {
		return false, errors.New("campaign snapshot task state is missing run_id")
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if time.Now().Add(time.Second).After(timeoutAt) {
			return false, nil
		}
		completed, err := w.snapshots.ProcessNextPage(ctx, task.WorkspaceID, task.State.SnapshotCampaign.RunID)
		if err != nil {
			return false, err
		}
		run, err := w.snapshots.repository.GetCampaignRun(ctx, task.WorkspaceID, task.State.SnapshotCampaign.RunID)
		if err != nil {
			return false, err
		}
		task.State.SnapshotCampaign.SnapshotCount = run.SnapshotCount
		if completed {
			task.Progress = 100
			return true, nil
		}
	}
}
