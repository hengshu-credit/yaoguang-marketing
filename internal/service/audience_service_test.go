package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
)

type audienceRepositoryStub struct {
	audiences map[string]domain.Audience
	versions  map[string]domain.AudienceVersion
}

func (audienceRepositoryStub) CreateAudience(context.Context, string, domain.Audience, domain.AudienceVersion) error {
	return nil
}
func (s audienceRepositoryStub) GetAudience(_ context.Context, _, id string) (*domain.Audience, error) {
	item, ok := s.audiences[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &item, nil
}
func (s audienceRepositoryStub) ListAudiences(context.Context, string, int, int) ([]domain.Audience, int, error) {
	return nil, 0, nil
}
func (s audienceRepositoryStub) GetAudienceVersion(_ context.Context, _, id string, _ int) (*domain.AudienceVersion, error) {
	item, ok := s.versions[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &item, nil
}
func (audienceRepositoryStub) SaveAudienceVersion(context.Context, string, string, domain.AudienceExpression) (*domain.AudienceVersion, error) {
	return &domain.AudienceVersion{}, nil
}
func (audienceRepositoryStub) PreviewAudience(context.Context, string, domain.AudienceExpression, int) ([]domain.CustomerSummary, int64, error) {
	return nil, 0, nil
}
func (audienceRepositoryStub) BuildAudience(context.Context, string, string, int) (string, int64, error) {
	return "", 0, nil
}
func (audienceRepositoryStub) GetAudienceBuild(context.Context, string, string) (*domain.AudienceBuild, error) {
	return nil, nil
}
func (audienceRepositoryStub) ListAudienceMembers(context.Context, string, string, string, int) ([]domain.CustomerSummary, string, error) {
	return nil, "", nil
}
func (audienceRepositoryStub) ArchiveAudience(context.Context, string, string) error { return nil }

func TestAudienceServiceRejectsDirectReferenceCycle(t *testing.T) {
	service, _ := NewAudienceService(audienceRepositoryStub{})
	_, err := service.UpdateDefinition(context.Background(), "workspace-1", "audience-1", domain.AudienceExpression{LeafType: domain.AudienceLeafAudience, RefID: "audience-1"})
	assert.ErrorContains(t, err, "reference itself")
}

func TestAudienceServiceRejectsTransitiveReferenceCycle(t *testing.T) {
	repository := audienceRepositoryStub{
		audiences: map[string]domain.Audience{
			"audience-b": {ID: "audience-b", ActiveVersion: 1},
			"audience-c": {ID: "audience-c", ActiveVersion: 1},
		},
		versions: map[string]domain.AudienceVersion{
			"audience-b": {Definition: domain.AudienceExpression{LeafType: domain.AudienceLeafAudience, RefID: "audience-c"}},
			"audience-c": {Definition: domain.AudienceExpression{LeafType: domain.AudienceLeafAudience, RefID: "audience-a"}},
		},
	}
	service, _ := NewAudienceService(repository)
	_, err := service.UpdateDefinition(context.Background(), "workspace-1", "audience-a", domain.AudienceExpression{LeafType: domain.AudienceLeafAudience, RefID: "audience-b"})
	assert.ErrorContains(t, err, "dependency cycle")
}
