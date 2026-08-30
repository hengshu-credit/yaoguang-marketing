package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/realtimecache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type frequencyPolicyRepositoryStub struct {
	policies    []domain.FrequencyPolicy
	saved       domain.FrequencyDecision
	savedPolicy domain.FrequencyPolicy
}

func (s *frequencyPolicyRepositoryStub) SaveFrequencyPolicy(_ context.Context, _ string, policy domain.FrequencyPolicy) error {
	s.savedPolicy = policy
	return nil
}
func (s *frequencyPolicyRepositoryStub) ListFrequencyPolicies(context.Context, string) ([]domain.FrequencyPolicy, error) {
	return s.policies, nil
}
func (s *frequencyPolicyRepositoryStub) ResolveFrequencyPolicies(context.Context, string, string, string, string) ([]domain.FrequencyPolicy, error) {
	return s.policies, nil
}
func (s *frequencyPolicyRepositoryStub) SaveFrequencyDecision(_ context.Context, _ string, decision domain.FrequencyDecision) error {
	s.saved = decision
	return nil
}

func TestFrequencyPolicyServicePersistsLayerThatDenied(t *testing.T) {
	triggerID := "22222222-2222-4222-8222-222222222222"
	repository := &frequencyPolicyRepositoryStub{policies: []domain.FrequencyPolicy{{
		ID: triggerID, Version: 2, Name: "事件触发频控", Scope: domain.FrequencyScopeTrigger, ScopeRef: "automation-1:event",
		Channel: "email", MaxEvents: 1, WindowKind: domain.FrequencyWindowSliding, WindowSeconds: 3600,
		DenyAction: domain.FrequencyActionSuppress, Enabled: true,
	}}}
	store := &fakeMultiFrequencyStore{multiResult: realtimecache.MultiWindowResult{Allowed: false, DeniedPolicyID: triggerID + ":v2"}}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)
	service, err := NewFrequencyPolicyService(repository, limiter)
	require.NoError(t, err)
	decision, err := service.Evaluate(context.Background(), FrequencyEvaluationRequest{
		WorkspaceID: "workspace-1", CustomerID: "11111111-1111-4111-8111-111111111111", Channel: "email",
		EffectKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TriggerRef: "automation-1:event", OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, domain.FrequencyScopeTrigger, decision.MatchedScope)
	assert.Equal(t, decision, repository.saved)
}

func TestFrequencyPolicyServiceDefersAndAuditsRedisFailure(t *testing.T) {
	repository := &frequencyPolicyRepositoryStub{policies: []domain.FrequencyPolicy{{
		ID: "33333333-3333-4333-8333-333333333333", Version: 1, Name: "全局频控", Scope: domain.FrequencyScopeWorkspaceGlobal,
		Channel: "sms", MaxEvents: 2, WindowKind: domain.FrequencyWindowSliding, WindowSeconds: 3600,
		DenyAction: domain.FrequencyActionDefer, Enabled: true,
	}}}
	store := &fakeMultiFrequencyStore{multiErr: assert.AnError}
	limiter, err := NewFrequencyLimiter(store)
	require.NoError(t, err)
	service, err := NewFrequencyPolicyService(repository, limiter)
	require.NoError(t, err)
	decision, err := service.Evaluate(context.Background(), FrequencyEvaluationRequest{WorkspaceID: "workspace-1", CustomerID: "customer-1", Channel: "sms", EffectKey: "effect-1", OccurredAt: time.Now().UTC()})
	require.Error(t, err)
	assert.True(t, decision.Deferred)
	assert.Contains(t, repository.saved.Reason, "frequency control unavailable")
}

func TestFrequencyPolicyManagementCreatesImmutableVersions(t *testing.T) {
	id := "33333333-3333-4333-8333-333333333333"
	repository := &frequencyPolicyRepositoryStub{policies: []domain.FrequencyPolicy{{ID: id, Version: 3}}}
	limiter, err := NewFrequencyLimiter(&fakeMultiFrequencyStore{})
	require.NoError(t, err)
	svc, err := NewFrequencyPolicyService(repository, limiter)
	require.NoError(t, err)
	policy, err := svc.SaveFrequencyPolicy(context.Background(), domain.SaveFrequencyPolicyRequest{
		WorkspaceID: "workspace-1", ID: id, Name: "全量邮件频控", Scope: domain.FrequencyScopeWorkspaceGlobal,
		Channel: "email", MaxEvents: 2, WindowKind: domain.FrequencyWindowCalendar, WindowSeconds: 86400,
		Timezone: "Asia/Shanghai", DenyAction: domain.FrequencyActionSuppress, Enabled: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 4, policy.Version)
	assert.Equal(t, *policy, repository.savedPolicy)
}
