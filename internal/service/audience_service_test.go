package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
func (audienceRepositoryStub) ListAudienceMembers(context.Context, string, domain.AudienceMemberQuery) ([]domain.AudienceMember, string, error) {
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

func TestAudienceServiceGetReturnsTheActiveVersionDefinition(t *testing.T) {
	definition := domain.AudienceExpression{Condition: &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
			FieldName: "profile_status", FieldType: "string", Operator: "equals", StringValues: []string{"unpaid"},
		}}}},
	}}
	repository := audienceRepositoryStub{
		audiences: map[string]domain.Audience{"audience-1": {ID: "audience-1", ActiveVersion: 3}},
		versions:  map[string]domain.AudienceVersion{"audience-1": {AudienceID: "audience-1", Version: 3, Definition: definition}},
	}
	service, err := NewAudienceService(repository)
	require.NoError(t, err)

	item, err := service.Get(context.Background(), "workspace-1", "audience-1")
	require.NoError(t, err)
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"definition":{"condition"`)
	assert.Contains(t, string(payload), `"profile_status"`)
}

type audienceRuntimeRepositoryStub struct {
	audienceRepositoryStub
	buildVersion int
	matchVersion int
	matchResult  bool
	matchErr     error
}

func (s *audienceRuntimeRepositoryStub) BuildAudienceSnapshot(_ context.Context, _, audienceID string, version int) (*domain.AudienceBuild, error) {
	s.buildVersion = version
	return &domain.AudienceBuild{ID: "build-1", AudienceID: audienceID, AudienceVersion: version, Status: "completed", MemberCount: 2}, nil
}

func (s *audienceRuntimeRepositoryStub) MatchesAudienceCustomer(_ context.Context, _, _ string, version int, _ string) (bool, error) {
	s.matchVersion = version
	return s.matchResult, s.matchErr
}

func TestAudienceServiceResolveLatestAndBuildReadsActiveVersionAtExecution(t *testing.T) {
	repository := &audienceRuntimeRepositoryStub{audienceRepositoryStub: audienceRepositoryStub{
		audiences: map[string]domain.Audience{"audience-1": {ID: "audience-1", ActiveVersion: 7}},
	}}
	service, err := NewAudienceService(repository)
	require.NoError(t, err)

	build, err := service.ResolveLatestAndBuild(context.Background(), "workspace-1", "audience-1")
	require.NoError(t, err)
	assert.Equal(t, 7, repository.buildVersion)
	assert.Equal(t, 7, build.AudienceVersion)
}

func TestAudienceServiceMatchesCustomerKeepsFalseSeparateFromErrors(t *testing.T) {
	repository := &audienceRuntimeRepositoryStub{matchResult: false}
	service, err := NewAudienceService(repository)
	require.NoError(t, err)

	matches, err := service.MatchesCustomer(context.Background(), "workspace-1", "audience-1", 5, "customer-1")
	require.NoError(t, err)
	assert.False(t, matches)
	assert.Equal(t, 5, repository.matchVersion)

	repository.matchErr = errors.New("database unavailable")
	_, err = service.MatchesCustomer(context.Background(), "workspace-1", "audience-1", 5, "customer-1")
	assert.ErrorContains(t, err, "database unavailable")
}
