package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type audienceBuildRunnerStub struct {
	builds []domain.AudienceBuild
	calls  int
}

func (s *audienceBuildRunnerStub) StartAudienceBuild(context.Context, string, string, int) (*domain.AudienceBuild, error) {
	return nil, nil
}

func (s *audienceBuildRunnerStub) ProcessAudienceBuildChunk(context.Context, string, string, int) (*domain.AudienceBuild, bool, error) {
	build := s.builds[s.calls]
	s.calls++
	return &build, build.Status == "completed", nil
}

func TestAudienceBuildWorkerContinuesKeysetPagesUntilCompleted(t *testing.T) {
	runner := &audienceBuildRunnerStub{builds: []domain.AudienceBuild{
		{ID: "build-1", Status: "building", MemberCount: 5_000},
		{ID: "build-1", Status: "completed", MemberCount: 7_250},
	}}
	worker, err := NewAudienceBuildWorker(runner, 5_000)
	require.NoError(t, err)
	task := &domain.Task{WorkspaceID: "workspace-1", State: &domain.TaskState{BuildAudience: &domain.BuildAudienceState{BuildID: "build-1"}}}
	completed, err := worker.Process(context.Background(), task, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 2, runner.calls)
	assert.Equal(t, int64(7_250), task.State.BuildAudience.MemberCount)
	assert.Equal(t, 100.0, task.Progress)
}
