package service

import (
	"context"
	"errors"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type importJobChunkRunner interface {
	ProcessNextChunkInternal(context.Context, string, string) (int, error)
	GetInternal(context.Context, string, string) (*domain.ImportJob, error)
}

// ImportJobWorker connects customer import jobs to the generic durable task
// scheduler. Row leases live in import_job_rows, so a process restart simply
// resumes the same task and safely reclaims expired chunks.
type ImportJobWorker struct {
	runner importJobChunkRunner
}

func NewImportJobWorker(runner importJobChunkRunner) (*ImportJobWorker, error) {
	if runner == nil {
		return nil, errors.New("import job runner is required")
	}
	return &ImportJobWorker{runner: runner}, nil
}

func (w *ImportJobWorker) CanProcess(taskType string) bool {
	return taskType == domain.ImportCustomersTaskType
}

func (w *ImportJobWorker) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error) {
	if task == nil || task.State == nil || task.State.ImportCustomers == nil || task.State.ImportCustomers.JobID == "" {
		return false, errors.New("import task state is missing job_id")
	}
	state := task.State.ImportCustomers
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if time.Now().Add(time.Second).After(timeoutAt) {
			return false, nil
		}
		processed, err := w.runner.ProcessNextChunkInternal(ctx, task.WorkspaceID, state.JobID)
		if err != nil {
			return false, err
		}
		job, err := w.runner.GetInternal(ctx, task.WorkspaceID, state.JobID)
		if err != nil {
			return false, err
		}
		state.TotalRows = job.Counters.Total
		state.ProcessedRows = job.Counters.Succeeded + job.Counters.Failed
		if state.TotalRows > 0 {
			task.Progress = float64(state.ProcessedRows) / float64(state.TotalRows) * 100
		}
		switch job.Status {
		case domain.ImportJobCompleted, domain.ImportJobCancelled, domain.ImportJobRejected:
			return true, nil
		}
		if processed == 0 {
			// Another worker still owns a non-expired lease. Re-queue without
			// spinning; the task scheduler will try again after the lease moves.
			return false, nil
		}
	}
}
