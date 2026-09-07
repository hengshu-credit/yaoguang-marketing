package service

import (
	"context"

	"errors"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type currentCampaignReader interface {
	GetCurrentCampaignConfiguration(context.Context, string, string) (*domain.CampaignConfiguration, error)
}
type currentCampaignRunWriter interface {
	CreateCampaignRunWithSnapshot(context.Context, string, domain.CampaignRun, *domain.ExecutionSnapshot) error
}

func (s *CampaignSnapshotService) SetExecutionSnapshotService(executions *ExecutionSnapshotService) {
	s.executions = executions
}
func (s *CampaignSnapshotService) startCurrent(ctx context.Context, ws, id, buildID string) (*domain.CampaignRun, error) {
	reader, ok := s.repository.(currentCampaignReader)
	if !ok {
		return nil, errors.New("current campaign configuration is unavailable")
	}
	config, err := reader.GetCurrentCampaignConfiguration(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	return s.StartConfiguration(ctx, ws, id, *config, buildID)
}

// StartConfiguration freezes the exact execution input and atomically persists
// the run and its snapshot. No CampaignVersion record is created.
func (s *CampaignSnapshotService) StartConfiguration(ctx context.Context, ws, id string, config domain.CampaignConfiguration, buildID string) (*domain.CampaignRun, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateTracking(ctx, ws, config); err != nil {
		return nil, err
	}
	writer, ok := s.repository.(currentCampaignRunWriter)
	if !ok {
		return nil, errors.New("atomic campaign snapshot storage is unavailable")
	}
	if config.AudienceID != "" && buildID == "" {
		resolver, ok := s.repository.(campaignAudienceBuildResolver)
		if !ok {
			return nil, errors.New("audience build resolver unavailable")
		}
		var err error
		buildID, err = resolver.GetCompletedAudienceBuildID(ctx, ws, config.AudienceID, config.AudienceVersion)
		if err != nil {
			return nil, err
		}
	}
	run := &domain.CampaignRun{ID: uuid.NewString(), CampaignID: id, AudienceID: config.AudienceID, AudienceVersion: config.AudienceVersion, AudienceBuildID: buildID, Status: "snapshotting", RunSeed: uuid.NewString(), NextOrdinal: 1, CreatedAt: s.now().UTC()}
	frozen, err := domain.SanitizeExecutionJSON(config)
	if err != nil {
		return nil, err
	}
	run.ExecutionConfiguration = frozen
	refs := []ExecutionTemplateReference{}
	for _, variant := range config.Variants {
		if variant.TemplateID != "" {
			refs = append(refs, ExecutionTemplateReference{ID: variant.TemplateID, Version: variant.TemplateVersion})
		}
	}
	var snapshot *domain.ExecutionSnapshot
	if s.executions != nil {
		snapshot, err = s.executions.Build(ctx, ws, "campaign", run.ID, id, config, refs)
	} else if len(refs) > 0 {
		return nil, errors.New("execution material archiver is unavailable")
	} else {
		snapshot, err = domain.NewExecutionSnapshot("campaign", run.ID, config, nil)
		if snapshot != nil {
			snapshot.SourceID = id
		}
	}
	if err != nil {
		return nil, err
	}
	snapshot.CreatedAt = run.CreatedAt
	if err = writer.CreateCampaignRunWithSnapshot(ctx, ws, *run, snapshot); err != nil {
		return nil, err
	}
	return run, nil
}
func (s *CampaignSnapshotService) SetBusinessDefinitions(d domain.BusinessDefinitionRepository) {
	s.businessDefinitions = d
}
func (s *CampaignSnapshotService) validateTracking(ctx context.Context, ws string, c domain.CampaignConfiguration) error {
	raw, err := domain.SanitizeExecutionJSON(c)
	if err != nil {
		return err
	}
	plan, err := domain.ExecutionTrackingPlan(raw)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	return ValidateTaskTracking(ctx, ws, s.businessDefinitions, plan)
}
