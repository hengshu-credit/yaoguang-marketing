package service

import (
	"context"
	"errors"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AudienceBuildWorker struct {
	runner    AudienceBuildRunner
	chunkSize int
}

func NewAudienceBuildWorker(runner AudienceBuildRunner, chunkSize int) (*AudienceBuildWorker, error) {
	if runner == nil || chunkSize <= 0 {
		return nil, errors.New("audience build runner and positive chunk size are required")
	}
	return &AudienceBuildWorker{runner: runner, chunkSize: chunkSize}, nil
}

func (w *AudienceBuildWorker) CanProcess(taskType string) bool {
	return taskType == domain.BuildAudienceTaskType
}

func (w *AudienceBuildWorker) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	if task == nil || task.State == nil || task.State.BuildAudience == nil || task.State.BuildAudience.BuildID == "" {
		return false, errors.New("audience build task state is missing build_id")
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if time.Now().Add(time.Second).After(timeoutAt) {
			return false, nil
		}
		build, completed, err := w.runner.ProcessAudienceBuildChunk(ctx, task.WorkspaceID, task.State.BuildAudience.BuildID, w.chunkSize)
		if err != nil {
			return false, err
		}
		task.State.BuildAudience.MemberCount = build.MemberCount
		if completed {
			task.Progress = 100
			return true, nil
		}
	}
}
