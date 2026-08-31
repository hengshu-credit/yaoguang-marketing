package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type automationAudienceRunRepositoryStub struct {
	automation     *domain.Automation
	runtimeVersion int
	buildID        string
	version        int
	enrolled       int64
}

func (s *automationAudienceRunRepositoryStub) GetByID(context.Context, string, string) (*domain.Automation, error) {
	copy := *s.automation
	return &copy, nil
}

func (s *automationAudienceRunRepositoryStub) GetAutomationRuntimeVersion(context.Context, string, string) (int, error) {
	return s.runtimeVersion, nil
}

func (s *automationAudienceRunRepositoryStub) EnrollAudienceBuild(_ context.Context, _, _, _ string, automationVersion int, _ string, _ int, buildID string) (int64, error) {
	s.buildID = buildID
	s.version = automationVersion
	return s.enrolled, nil
}

type automationAudienceExecutionStub struct{ build domain.AudienceBuild }

func (s *automationAudienceExecutionStub) ResolveLatestAndBuildInternal(context.Context, string, string) (*domain.AudienceBuild, error) {
	copy := s.build
	return &copy, nil
}

func TestAutomationAudienceRunStartsLiveAutomationFromResolvedCandidateBuild(t *testing.T) {
	repository := &automationAudienceRunRepositoryStub{automation: &domain.Automation{
		ID: "automation-1", RootNodeID: "trigger-1", Status: domain.AutomationStatusLive,
		Nodes: []*domain.AutomationNode{{ID: "trigger-1", Type: domain.NodeTypeTrigger}},
	}, runtimeVersion: 4, enrolled: 2}
	audiences := &automationAudienceExecutionStub{build: domain.AudienceBuild{
		ID: "build-7", AudienceID: "audience-1", AudienceVersion: 7, Status: "completed", MemberCount: 3,
	}}
	service, err := NewAutomationAudienceRunService(repository, audiences, nil)
	require.NoError(t, err)

	result, err := service.Start(context.Background(), AutomationAudienceRunRequest{
		WorkspaceID: "workspace-1", AutomationID: "automation-1", AudienceID: "audience-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "build-7", repository.buildID)
	assert.Equal(t, 4, repository.version)
	assert.Equal(t, "build-7", result.BuildID)
	assert.Equal(t, 7, result.AudienceVersion)
	assert.Equal(t, int64(3), result.CandidateCount)
	assert.Equal(t, int64(2), result.EnrolledCount)
}
