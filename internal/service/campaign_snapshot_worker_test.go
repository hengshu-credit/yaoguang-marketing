package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastCampaignVariantsAlwaysTotalOneHundredPercent(t *testing.T) {
	broadcast := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{
		{TemplateID: "a"}, {TemplateID: "b"}, {TemplateID: "c"},
	}}}
	variants := broadcastCampaignVariants(broadcast)
	total := 0
	for _, item := range variants {
		total += item.WeightBP
	}
	assert.Equal(t, 10_000, total)
}

type campaignSnapshotWorkerRepository struct {
	calls int
	run   domain.CampaignRun
}

func (s *campaignSnapshotWorkerRepository) CreateCampaign(context.Context, string, domain.Campaign, domain.CampaignVersion) error {
	return nil
}
func (s *campaignSnapshotWorkerRepository) GetCampaign(context.Context, string, string) (*domain.Campaign, error) {
	return nil, nil
}
func (s *campaignSnapshotWorkerRepository) ListCampaigns(context.Context, string, int, int) ([]domain.Campaign, int, error) {
	return nil, 0, nil
}
func (s *campaignSnapshotWorkerRepository) GetCampaignVersion(context.Context, string, string, int) (*domain.CampaignVersion, error) {
	return &domain.CampaignVersion{CampaignID: "campaign-1", Version: 1, AudienceID: "audience-1", AudienceVersion: 1,
		Channel: "email", Variants: []domain.CampaignVariant{{ID: "a", WeightBP: 10_000}}}, nil
}
func (s *campaignSnapshotWorkerRepository) CreateCampaignRun(context.Context, string, domain.CampaignRun) error {
	return nil
}
func (s *campaignSnapshotWorkerRepository) GetCampaignRun(context.Context, string, string) (*domain.CampaignRun, error) {
	copy := s.run
	return &copy, nil
}
func (s *campaignSnapshotWorkerRepository) ListAudienceMembers(context.Context, string, string, int, string, int) ([]domain.CampaignAudienceMember, string, error) {
	s.calls++
	if s.calls == 1 {
		return []domain.CampaignAudienceMember{{CustomerID: "customer-1", BuildID: "build-1"}}, "customer-1", nil
	}
	return nil, "", nil
}
func (s *campaignSnapshotWorkerRepository) AppendCampaignSnapshots(context.Context, string, string, []domain.CampaignRecipientSnapshot) (int64, error) {
	s.run.SnapshotCount++
	s.run.NextOrdinal++
	return 1, nil
}
func (s *campaignSnapshotWorkerRepository) CompleteCampaignSnapshot(context.Context, string, string, int64) error {
	s.run.Status = "dispatching"
	return nil
}
func (s *campaignSnapshotWorkerRepository) ListCampaignSnapshots(context.Context, string, string, int64, int) ([]domain.CampaignRecipientSnapshot, int64, error) {
	return nil, 0, nil
}

func TestCampaignSnapshotWorkerCompletesBeforeDispatch(t *testing.T) {
	repo := &campaignSnapshotWorkerRepository{run: domain.CampaignRun{ID: "run-1", CampaignID: "campaign-1", CampaignVersion: 1, Status: "snapshotting", RunSeed: "seed", NextOrdinal: 1}}
	snapshots, err := NewCampaignSnapshotService(repo, 1)
	require.NoError(t, err)
	worker, err := NewCampaignSnapshotWorker(snapshots)
	require.NoError(t, err)
	task := &domain.Task{WorkspaceID: "workspace-1", State: &domain.TaskState{SnapshotCampaign: &domain.SnapshotCampaignState{RunID: "run-1"}}}
	completed, err := worker.Process(context.Background(), task, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, "dispatching", repo.run.Status)
	assert.Equal(t, int64(1), task.State.SnapshotCampaign.SnapshotCount)
	assert.Equal(t, 100.0, task.Progress)
}
