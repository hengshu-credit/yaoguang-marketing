package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type importChunkRunnerStub struct {
	jobs      []domain.ImportJob
	processed []int
	calls     int
}

func (s *importChunkRunnerStub) ProcessNextChunkInternal(context.Context, string, string) (int, error) {
	value := s.processed[s.calls]
	s.calls++
	return value, nil
}

func (s *importChunkRunnerStub) GetInternal(context.Context, string, string) (*domain.ImportJob, error) {
	job := s.jobs[s.calls-1]
	return &job, nil
}

func TestImportJobWorkerContinuesUntilEveryRowIsTerminal(t *testing.T) {
	runner := &importChunkRunnerStub{
		processed: []int{2_000, 500},
		jobs: []domain.ImportJob{
			{Status: domain.ImportJobProcessing, Counters: domain.ImportCounters{Total: 2_500, Pending: 500, Succeeded: 2_000}},
			{Status: domain.ImportJobCompleted, Counters: domain.ImportCounters{Total: 2_500, Succeeded: 2_499, Failed: 1}},
		},
	}
	worker, err := NewImportJobWorker(runner)
	require.NoError(t, err)
	task := &domain.Task{WorkspaceID: "workspace-1", State: &domain.TaskState{ImportCustomers: &domain.ImportCustomersState{JobID: "job-1"}}}
	completed, err := worker.Process(context.Background(), task, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 2, runner.calls)
	assert.Equal(t, int64(2_500), task.State.ImportCustomers.ProcessedRows)
	assert.Equal(t, 100.0, task.Progress)
}

func TestImportJobWorkerRequeuesWhenRowsRemainLeased(t *testing.T) {
	runner := &importChunkRunnerStub{
		processed: []int{0},
		jobs:      []domain.ImportJob{{Status: domain.ImportJobProcessing, Counters: domain.ImportCounters{Total: 10, Processing: 10}}},
	}
	worker, err := NewImportJobWorker(runner)
	require.NoError(t, err)
	task := &domain.Task{WorkspaceID: "workspace-1", State: &domain.TaskState{ImportCustomers: &domain.ImportCustomersState{JobID: "job-1"}}}
	completed, err := worker.Process(context.Background(), task, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, completed)
	assert.Zero(t, task.Progress)
}
