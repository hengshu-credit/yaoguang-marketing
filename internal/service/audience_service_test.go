package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
)

type audienceRepositoryStub struct{}

func (audienceRepositoryStub) CreateAudience(context.Context, string, domain.Audience, domain.AudienceVersion) error {
	return nil
}
func (audienceRepositoryStub) GetAudience(context.Context, string, string) (*domain.Audience, error) {
	return nil, nil
}
func (audienceRepositoryStub) GetAudienceVersion(context.Context, string, string, int) (*domain.AudienceVersion, error) {
	return nil, nil
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

func TestAudienceServiceRejectsDirectReferenceCycle(t *testing.T) {
	service, _ := NewAudienceService(audienceRepositoryStub{})
	_, err := service.UpdateDefinition(context.Background(), "workspace-1", "audience-1", domain.AudienceExpression{LeafType: domain.AudienceLeafAudience, RefID: "audience-1"})
	assert.ErrorContains(t, err, "reference itself")
}
