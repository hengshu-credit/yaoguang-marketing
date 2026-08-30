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

func NewCampaignSnapshotService(repository domain.CampaignRepository, pageSize int) (*CampaignSnapshotService, error) {
	if repository == nil || pageSize <= 0 {
		return nil, errors.New("campaign repository and positive page size are required")
	}
	return &CampaignSnapshotService{repository: repository, pageSize: pageSize, now: time.Now}, nil
}

func (s *CampaignSnapshotService) Start(ctx context.Context, workspaceID, campaignID string, version int) (*domain.CampaignRun, error) {
	campaignVersion, err := s.repository.GetCampaignVersion(ctx, workspaceID, campaignID, version)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	run := &domain.CampaignRun{ID: uuid.New().String(), CampaignID: campaignID, CampaignVersion: version,
		Status: "snapshotting", RunSeed: uuid.New().String(), NextOrdinal: 1, CreatedAt: now}
	if err := s.repository.CreateCampaignRun(ctx, workspaceID, *run); err != nil {
		return nil, err
	}
	after := ""
	ordinal := int64(1)
	for {
		customerIDs, next, err := s.repository.ListAudienceMemberIDs(ctx, workspaceID, campaignVersion.AudienceID, campaignVersion.AudienceVersion, after, s.pageSize)
		if err != nil {
			return nil, err
		}
		if len(customerIDs) == 0 {
			break
		}
		snapshots := make([]domain.CampaignRecipientSnapshot, 0, len(customerIDs))
		for _, customerID := range customerIDs {
			variant, err := campaignVersion.AssignVariant(customerID, run.RunSeed)
			if err != nil {
				return nil, err
			}
			snapshots = append(snapshots, domain.CampaignRecipientSnapshot{RunID: run.ID, Ordinal: ordinal,
				CustomerID: customerID, Variant: variant, CreatedAt: now})
			ordinal++
		}
		if _, err := s.repository.AppendCampaignSnapshots(ctx, workspaceID, run.ID, snapshots); err != nil {
			return nil, err
		}
		after = next
		if len(customerIDs) < s.pageSize {
			break
		}
	}
	run.SnapshotCount = ordinal - 1
	run.NextOrdinal = ordinal
	run.Status = "dispatching"
	if err := s.repository.CompleteCampaignSnapshot(ctx, workspaceID, run.ID, run.SnapshotCount); err != nil {
		return nil, err
	}
	return run, nil
}
