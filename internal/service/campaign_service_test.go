package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executionAudienceStub struct {
	requestedAudienceID string
	build               domain.AudienceBuild
}

func (s *executionAudienceStub) ResolveLatestAndBuildInternal(_ context.Context, _, audienceID string) (*domain.AudienceBuild, error) {
	s.requestedAudienceID = audienceID
	copy := s.build
	return &copy, nil
}

type recordingCampaignTaskScheduler struct{ tasks []*domain.Task }

func (s *recordingCampaignTaskScheduler) CreateTask(_ context.Context, _ string, task *domain.Task) error {
	s.tasks = append(s.tasks, task)
	return nil
}

func TestCampaignServicePrepareBroadcastExecutionResolvesLatestAudienceAndExactBuild(t *testing.T) {
	repository := &campaignRepositoryStub{}
	snapshots, err := NewCampaignSnapshotService(repository, 100)
	require.NoError(t, err)
	service, err := NewCampaignService(repository, snapshots)
	require.NoError(t, err)
	audiences := &executionAudienceStub{build: domain.AudienceBuild{
		ID: "build-7", AudienceID: "audience-1", AudienceVersion: 7, Status: "completed", MemberCount: 2,
	}}
	scheduler := &recordingCampaignTaskScheduler{}
	service.SetAudienceExecutionService(audiences)
	service.SetTaskScheduler(scheduler)

	run, err := service.PrepareBroadcastExecution(context.Background(), "workspace-1", &domain.Broadcast{
		ID: "broadcast-1", Name: "还款提醒", ChannelType: "email",
		Audience: domain.AudienceSettings{AudienceID: "audience-1", AudienceVersion: 2, AudienceBuildID: "old-build"},
	})
	require.NoError(t, err)
	assert.Equal(t, "audience-1", audiences.requestedAudienceID)
	assert.Equal(t, 7, repository.version.AudienceVersion)
	assert.Equal(t, "build-7", run.AudienceBuildID)
	assert.Equal(t, 7, run.AudienceVersion)
	require.Len(t, scheduler.tasks, 1)
	assert.Equal(t, run.ID, scheduler.tasks[0].State.SnapshotCampaign.RunID)
}
