package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type CampaignSnapshotService struct {
	repository domain.CampaignRepository
	pageSize   int
	now        func() time.Time
}

type campaignAudienceBuildResolver interface {
	GetCompletedAudienceBuildID(context.Context, string, string, int) (string, error)
}

func NewCampaignSnapshotService(repository domain.CampaignRepository, pageSize int) (*CampaignSnapshotService, error) {
	if repository == nil || pageSize <= 0 {
		return nil, errors.New("campaign repository and positive page size are required")
	}
	return &CampaignSnapshotService{repository: repository, pageSize: pageSize, now: time.Now}, nil
}

func (s *CampaignSnapshotService) Start(ctx context.Context, workspaceID, campaignID string, version int) (*domain.CampaignRun, error) {
	run, err := s.StartAsync(ctx, workspaceID, campaignID, version)
	if err != nil {
		return nil, err
	}
	for {
		completed, err := s.ProcessNextPage(ctx, workspaceID, run.ID)
		if err != nil {
			return nil, err
		}
		if completed {
			return s.repository.GetCampaignRun(ctx, workspaceID, run.ID)
		}
	}
}

func (s *CampaignSnapshotService) StartAsync(ctx context.Context, workspaceID, campaignID string, version int) (*domain.CampaignRun, error) {
	return s.startAsync(ctx, workspaceID, campaignID, version, "", false)
}

func (s *CampaignSnapshotService) StartAsyncResolved(ctx context.Context, workspaceID, campaignID string, version int, audienceBuildID string) (*domain.CampaignRun, error) {
	return s.startAsync(ctx, workspaceID, campaignID, version, audienceBuildID, true)
}

func (s *CampaignSnapshotService) startAsync(ctx context.Context, workspaceID, campaignID string, version int, audienceBuildID string, requireResolvedBuild bool) (*domain.CampaignRun, error) {
	campaignVersion, err := s.repository.GetCampaignVersion(ctx, workspaceID, campaignID, version)
	if err != nil {
		return nil, err
	}
	if requireResolvedBuild && campaignVersion.AudienceID != "" && audienceBuildID == "" {
		return nil, errors.New("resolved audience build id is required")
	}
	if !requireResolvedBuild && campaignVersion.AudienceID != "" && audienceBuildID == "" {
		resolver, ok := s.repository.(campaignAudienceBuildResolver)
		if !ok {
			return nil, errors.New("campaign repository cannot resolve an audience build")
		}
		audienceBuildID, err = resolver.GetCompletedAudienceBuildID(
			ctx, workspaceID, campaignVersion.AudienceID, campaignVersion.AudienceVersion,
		)
		if err != nil {
			return nil, err
		}
	}
	now := s.now().UTC()
	run := &domain.CampaignRun{ID: uuid.New().String(), CampaignID: campaignID, CampaignVersion: version,
		AudienceID: campaignVersion.AudienceID, AudienceVersion: campaignVersion.AudienceVersion,
		AudienceBuildID: audienceBuildID,
		Status:          "snapshotting", RunSeed: uuid.New().String(), NextOrdinal: 1, CreatedAt: now}
	if err := s.repository.CreateCampaignRun(ctx, workspaceID, *run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *CampaignSnapshotService) ProcessNextPage(ctx context.Context, workspaceID, runID string) (bool, error) {
	run, err := s.repository.GetCampaignRun(ctx, workspaceID, runID)
	if err != nil {
		return false, err
	}
	if run.Status == "dispatching" || run.Status == "completed" {
		return true, nil
	}
	if run.Status != "snapshotting" {
		return false, errors.New("campaign run is not snapshotting")
	}
	version, err := s.repository.GetCampaignVersion(ctx, workspaceID, run.CampaignID, run.CampaignVersion)
	if err != nil {
		return false, err
	}
	members, _, err := s.repository.ListCampaignMembers(ctx, workspaceID, *version, run.AudienceBuildID, run.SnapshotLastCustomerID, s.pageSize)
	if err != nil {
		return false, err
	}
	if len(members) > 0 {
		snapshots := make([]domain.CampaignRecipientSnapshot, 0, len(members))
		ordinal := run.NextOrdinal
		for _, member := range members {
			variant, assignErr := version.AssignVariant(member.CustomerID, run.RunSeed)
			if assignErr != nil {
				return false, assignErr
			}
			snapshots = append(snapshots, domain.CampaignRecipientSnapshot{RunID: run.ID, Ordinal: ordinal,
				CustomerID: member.CustomerID, Variant: variant, SourceBuildID: member.BuildID, CreatedAt: s.now().UTC()})
			ordinal++
		}
		inserted, err := s.repository.AppendCampaignSnapshots(ctx, workspaceID, run.ID, snapshots)
		if err != nil {
			return false, err
		}
		run.SnapshotCount += inserted
	}
	completed := len(members) < s.pageSize
	if completed {
		if err := s.repository.CompleteCampaignSnapshot(ctx, workspaceID, run.ID, run.SnapshotCount); err != nil {
			return false, err
		}
	}
	return completed, nil
}
