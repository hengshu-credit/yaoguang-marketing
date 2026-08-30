package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type campaignRepositoryStub struct {
	version   domain.CampaignVersion
	members   []string
	snapshots []domain.CampaignRecipientSnapshot
	completed int64
}

func (s *campaignRepositoryStub) CreateCampaign(context.Context, string, domain.Campaign, domain.CampaignVersion) error {
	return nil
}
func (s *campaignRepositoryStub) GetCampaignVersion(context.Context, string, string, int) (*domain.CampaignVersion, error) {
	copy := s.version
	return &copy, nil
}
func (s *campaignRepositoryStub) CreateCampaignRun(context.Context, string, domain.CampaignRun) error {
	return nil
}
func (s *campaignRepositoryStub) ListAudienceMemberIDs(_ context.Context, _, _ string, _ int, after string, limit int) ([]string, string, error) {
	start := 0
	if after != "" {
		for index, id := range s.members {
			if id == after {
				start = index + 1
			}
		}
	}
	end := start + limit
	if end > len(s.members) {
		end = len(s.members)
	}
	page := append([]string(nil), s.members[start:end]...)
	next := ""
	if len(page) > 0 {
		next = page[len(page)-1]
	}
	return page, next, nil
}
func (s *campaignRepositoryStub) AppendCampaignSnapshots(_ context.Context, _, _ string, snapshots []domain.CampaignRecipientSnapshot) (int64, error) {
	s.snapshots = append(s.snapshots, snapshots...)
	return int64(len(snapshots)), nil
}
func (s *campaignRepositoryStub) CompleteCampaignSnapshot(_ context.Context, _, _ string, count int64) error {
	s.completed = count
	return nil
}

func TestCampaignSnapshotFreezesUniqueMembersAndStableVariants(t *testing.T) {
	repository := &campaignRepositoryStub{version: domain.CampaignVersion{CampaignID: "campaign-1", Version: 1,
		AudienceID: "audience-1", AudienceVersion: 2, Channel: "email",
		Variants: []domain.CampaignVariant{{ID: "a", WeightBP: 5000}, {ID: "b", WeightBP: 5000}}},
		members: []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}}
	service, err := NewCampaignSnapshotService(repository, 2)
	require.NoError(t, err)
	run, err := service.Start(context.Background(), "workspace-1", "campaign-1", 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), run.SnapshotCount)
	assert.Equal(t, int64(3), repository.completed)
	require.Len(t, repository.snapshots, 3)
	assert.Equal(t, int64(1), repository.snapshots[0].Ordinal)
	assert.Equal(t, int64(3), repository.snapshots[2].Ordinal)
	assert.NotEmpty(t, repository.snapshots[0].Variant)
}
