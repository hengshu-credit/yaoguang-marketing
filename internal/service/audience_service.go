package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AudienceService struct{ repository domain.AudienceRepository }

func NewAudienceService(repository domain.AudienceRepository) (*AudienceService, error) {
	if repository == nil {
		return nil, errors.New("audience repository is required")
	}
	return &AudienceService{repository: repository}, nil
}

type CreateAudienceRequest struct {
	WorkspaceID string
	Name        string
	Description string
	Kind        domain.AudienceKind
	Definition  domain.AudienceExpression
}

func (s *AudienceService) Create(ctx context.Context, request CreateAudienceRequest) (*domain.Audience, error) {
	if strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("workspace and audience name are required")
	}
	if err := request.Definition.Validate(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	audience := domain.Audience{ID: uuid.New().String(), Name: strings.TrimSpace(request.Name), Description: strings.TrimSpace(request.Description),
		Kind: request.Kind, ActiveVersion: 1, CreatedAt: now, UpdatedAt: now}
	if audience.Kind == "" {
		audience.Kind = domain.AudienceKindDynamic
	}
	hash, _ := request.Definition.VersionHash()
	version := domain.AudienceVersion{AudienceID: audience.ID, Version: 1, Definition: request.Definition, DefinitionHash: hash, CreatedAt: now}
	if err := s.repository.CreateAudience(ctx, request.WorkspaceID, audience, version); err != nil {
		return nil, err
	}
	return &audience, nil
}

func (s *AudienceService) UpdateDefinition(ctx context.Context, workspaceID, audienceID string, expression domain.AudienceExpression) (*domain.AudienceVersion, error) {
	if containsAudienceReference(expression, audienceID) {
		return nil, errors.New("audience cannot reference itself")
	}
	return s.repository.SaveAudienceVersion(ctx, workspaceID, audienceID, expression)
}

func (s *AudienceService) Preview(ctx context.Context, workspaceID string, expression domain.AudienceExpression) ([]domain.CustomerSummary, int64, error) {
	return s.repository.PreviewAudience(ctx, workspaceID, expression, 100)
}

func (s *AudienceService) Build(ctx context.Context, workspaceID, audienceID string, version int) (string, int64, error) {
	return s.repository.BuildAudience(ctx, workspaceID, audienceID, version)
}

func containsAudienceReference(expression domain.AudienceExpression, audienceID string) bool {
	if expression.LeafType == domain.AudienceLeafAudience && expression.RefID == audienceID {
		return true
	}
	for _, child := range expression.Children {
		if containsAudienceReference(child, audienceID) {
			return true
		}
	}
	return false
}
