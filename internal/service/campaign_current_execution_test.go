package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

type currentCampaignRepositoryStub struct {
	campaignRepositoryStub
	config   domain.CampaignConfiguration
	snapshot *domain.ExecutionSnapshot
}

func (r *currentCampaignRepositoryStub) GetCurrentCampaignConfiguration(context.Context, string, string) (*domain.CampaignConfiguration, error) {
	copy := r.config
	return &copy, nil
}
func (r *currentCampaignRepositoryStub) CreateCampaignRunWithSnapshot(_ context.Context, _ string, run domain.CampaignRun, snapshot *domain.ExecutionSnapshot) error {
	r.run = run
	r.snapshot = snapshot
	return nil
}
func TestCampaignCurrentExecutionNeedsNoVersionAndPreservesConfig(t *testing.T) {
	repo := &currentCampaignRepositoryStub{config: domain.CampaignConfiguration{ListID: "news", Channel: "email", Variants: []domain.CampaignVariant{{ID: "a", WeightBP: 10000}}, Settings: json.RawMessage(`{"filters":{"product":"A"}}`)}}
	s, err := NewCampaignSnapshotService(repo, 100)
	require.NoError(t, err)
	run, err := s.StartAsync(context.Background(), "ws", "campaign", 0)
	require.NoError(t, err)
	require.Zero(t, run.CampaignVersion)
	require.NotNil(t, repo.snapshot)
	repo.config.Settings = json.RawMessage(`{"filters":{"product":"B"}}`)
	require.Contains(t, string(repo.snapshot.Configuration), `"product":"A"`)
	require.Contains(t, string(run.ExecutionConfiguration), `"product":"A"`)
	_, err = s.ProcessNextPage(context.Background(), "ws", run.ID)
	require.NoError(t, err)
}
