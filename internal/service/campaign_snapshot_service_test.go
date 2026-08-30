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
	members   []domain.CampaignAudienceMember
	snapshots []domain.CampaignRecipientSnapshot
	completed int64
	run       domain.CampaignRun
}

func (s *campaignRepositoryStub) CreateCampaign(context.Context, string, domain.Campaign, domain.CampaignVersion) error {
	return nil
}
func (s *campaignRepositoryStub) GetCampaign(context.Context, string, string) (*domain.Campaign, error) {
	return &domain.Campaign{}, nil
}
func (s *campaignRepositoryStub) ListCampaigns(context.Context, string, int, int) ([]domain.Campaign, int, error) {
	return nil, 0, nil
}
func (s *campaignRepositoryStub) GetCampaignVersion(context.Context, string, string, int) (*domain.CampaignVersion, error) {
	copy := s.version
	return &copy, nil
}

func (s *campaignRepositoryStub) CreateCampaignRun(_ context.Context, _ string, run domain.CampaignRun) error {
	s.run = run
	return nil
}
func (s *campaignRepositoryStub) GetCampaignRun(context.Context, string, string) (*domain.CampaignRun, error) {
	copy := s.run
	return &copy, nil
}
func (s *campaignRepositoryStub) ListAudienceMembers(_ context.Context, _, _ string, _ int, after string, limit int) ([]domain.CampaignAudienceMember, string, error) {
	start := 0
	if after != "" {
		for index, member := range s.members {
			if member.CustomerID == after {
				start = index + 1
			}
		}
	}
	end := start + limit
	if end > len(s.members) {
		end = len(s.members)
	}
	page := append([]domain.CampaignAudienceMember(nil), s.members[start:end]...)
	next := ""
	if len(page) > 0 {
		next = page[len(page)-1].CustomerID
	}
	return page, next, nil
}
func (s *campaignRepositoryStub) AppendCampaignSnapshots(_ context.Context, _, _ string, snapshots []domain.CampaignRecipientSnapshot) (int64, error) {
	s.snapshots = append(s.snapshots, snapshots...)
	if len(snapshots) > 0 {
		s.run.SnapshotLastCustomerID = snapshots[len(snapshots)-1].CustomerID
		s.run.NextOrdinal = snapshots[len(snapshots)-1].Ordinal + 1
		s.run.SnapshotCount += int64(len(snapshots))
	}
	return int64(len(snapshots)), nil
}
func (s *campaignRepositoryStub) CompleteCampaignSnapshot(_ context.Context, _, _ string, count int64) error {
	s.completed = count
	s.run.SnapshotCount = count
	s.run.Status = "dispatching"
	return nil
}
func (s *campaignRepositoryStub) ListCampaignSnapshots(context.Context, string, string, int64, int) ([]domain.CampaignRecipientSnapshot, int64, error) {
	return nil, 0, nil
}

func TestCampaignSnapshotFreezesUniqueMembersAndStableVariants(t *testing.T) {
	repository := &campaignRepositoryStub{version: domain.CampaignVersion{CampaignID: "campaign-1", Version: 1,
		AudienceID: "audience-1", AudienceVersion: 2, Channel: "email",
		Variants: []domain.CampaignVariant{{ID: "a", WeightBP: 5000}, {ID: "b", WeightBP: 5000}}},
		members: []domain.CampaignAudienceMember{
			{CustomerID: "11111111-1111-4111-8111-111111111111", BuildID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
			{CustomerID: "22222222-2222-4222-8222-222222222222", BuildID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
			{CustomerID: "33333333-3333-4333-8333-333333333333", BuildID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}}}
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
	assert.Equal(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", repository.snapshots[0].SourceBuildID)
}
