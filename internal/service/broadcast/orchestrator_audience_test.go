package broadcast

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type audienceRunResolverStub struct {
	calls int
	run   *domain.CampaignRun
}

func (s *audienceRunResolverStub) PrepareBroadcastExecution(context.Context, string, *domain.Broadcast) (*domain.CampaignRun, error) {
	s.calls++
	return s.run, nil
}

func TestEnsureExecutionAudiencePersistsResolvedRunBeforeCounting(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := domainmocks.NewMockBroadcastRepository(ctrl)
	broadcast := &domain.Broadcast{ID: "broadcast-1", Audience: domain.AudienceSettings{AudienceID: "audience-1"}}
	resolver := &audienceRunResolverStub{run: &domain.CampaignRun{
		ID: "run-1", AudienceID: "audience-1", AudienceVersion: 7, AudienceBuildID: "build-7",
	}}
	repository.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(broadcast, nil)
	repository.EXPECT().UpdateBroadcast(gomock.Any(), broadcast).DoAndReturn(func(_ context.Context, got *domain.Broadcast) error {
		assert.Equal(t, "run-1", got.Audience.CampaignRunID)
		assert.Equal(t, 7, got.Audience.AudienceVersion)
		assert.Equal(t, "build-7", got.Audience.AudienceBuildID)
		return nil
	})
	orchestrator := &BroadcastOrchestrator{broadcastRepo: repository, audienceRunResolver: resolver}

	prepared, err := orchestrator.ensureExecutionAudience(context.Background(), "workspace-1", "broadcast-1")
	require.NoError(t, err)
	assert.True(t, prepared)
	assert.Equal(t, 1, resolver.calls)
}
