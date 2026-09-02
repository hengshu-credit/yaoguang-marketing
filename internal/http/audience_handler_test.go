package http

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type audienceMatchRepositoryStub struct {
	audience domain.Audience
	version  int
}

func (audienceMatchRepositoryStub) CreateAudience(context.Context, string, domain.Audience, domain.AudienceVersion) error {
	return nil
}
func (stub *audienceMatchRepositoryStub) GetAudience(context.Context, string, string) (*domain.Audience, error) {
	return &stub.audience, nil
}
func (audienceMatchRepositoryStub) ListAudiences(context.Context, string, int, int) ([]domain.Audience, int, error) {
	return nil, 0, nil
}
func (audienceMatchRepositoryStub) GetAudienceVersion(context.Context, string, string, int) (*domain.AudienceVersion, error) {
	return nil, sql.ErrNoRows
}
func (audienceMatchRepositoryStub) SaveAudienceVersion(context.Context, string, string, domain.AudienceExpression) (*domain.AudienceVersion, error) {
	return nil, nil
}
func (audienceMatchRepositoryStub) PreviewAudience(context.Context, string, domain.AudienceExpression, int) ([]domain.CustomerSummary, int64, error) {
	return nil, 0, nil
}
func (audienceMatchRepositoryStub) BuildAudience(context.Context, string, string, int) (string, int64, error) {
	return "", 0, nil
}
func (audienceMatchRepositoryStub) GetAudienceBuild(context.Context, string, string) (*domain.AudienceBuild, error) {
	return nil, nil
}
func (audienceMatchRepositoryStub) ListAudienceMembers(context.Context, string, domain.AudienceMemberQuery) ([]domain.AudienceMember, string, error) {
	return nil, "", nil
}
func (audienceMatchRepositoryStub) ArchiveAudience(context.Context, string, string) error { return nil }
func (audienceMatchRepositoryStub) BuildAudienceSnapshot(context.Context, string, string, int) (*domain.AudienceBuild, error) {
	return nil, nil
}
func (stub *audienceMatchRepositoryStub) MatchesAudienceCustomer(_ context.Context, _, _ string, version int, _ string) (bool, error) {
	stub.version = version
	return true, nil
}

func TestAudienceHandlerMatchesCustomerAgainstTheCurrentDefinition(t *testing.T) {
	repository := &audienceMatchRepositoryStub{audience: domain.Audience{
		ID: "11111111-1111-4111-8111-111111111111", Name: "Test audience",
		Kind: domain.AudienceKindDynamic, ActiveVersion: 4,
	}}
	audienceService, err := service.NewAudienceService(repository)
	require.NoError(t, err)
	handler := NewAudienceHandler(audienceService, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/audiences.matchCustomer", bytes.NewBufferString(`{
		"workspace_id":"workspace-1",
		"audience_id":"11111111-1111-4111-8111-111111111111",
		"customer_id":"22222222-2222-4222-8222-222222222222"
	}`))
	response := httptest.NewRecorder()

	handler.matchCustomer(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"audience_id":"11111111-1111-4111-8111-111111111111",
		"name":"Test audience",
		"kind":"dynamic",
		"audience_version":4,
		"matches":true
	}`, response.Body.String())
	assert.Equal(t, 4, repository.version)
}
