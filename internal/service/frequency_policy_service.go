package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type FrequencyEvaluationRequest = domain.FrequencyEvaluationRequest

type FrequencyPolicyService struct {
	repository domain.FrequencyPolicyRepository
	limiter    *FrequencyLimiter
	auth       domain.AuthService
	now        func() time.Time
}

func NewFrequencyPolicyService(repository domain.FrequencyPolicyRepository, limiter *FrequencyLimiter) (*FrequencyPolicyService, error) {
	if repository == nil || limiter == nil {
		return nil, fmt.Errorf("frequency policy repository and limiter are required")
	}
	return &FrequencyPolicyService{repository: repository, limiter: limiter, now: time.Now}, nil
}

func (s *FrequencyPolicyService) SetManagementAuth(auth domain.AuthService) {
	s.auth = auth
}

func (s *FrequencyPolicyService) authorizeManagement(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	if s.auth == nil {
		return ctx, nil
	}
	ctx, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, err
	}
	if !membership.HasPermission(domain.PermissionResourceBroadcasts, permission) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceBroadcasts, permission, "Broadcast access is required to manage frequency policies")
	}
	return ctx, nil
}

func (s *FrequencyPolicyService) ListFrequencyPolicies(ctx context.Context, workspaceID string) ([]domain.FrequencyPolicy, error) {
	var err error
	ctx, err = s.authorizeManagement(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.ListFrequencyPolicies(ctx, workspaceID)
}

func (s *FrequencyPolicyService) SaveFrequencyPolicy(ctx context.Context, request domain.SaveFrequencyPolicyRequest) (*domain.FrequencyPolicy, error) {
	if request.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	var err error
	ctx, err = s.authorizeManagement(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	policies, err := s.repository.ListFrequencyPolicies(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	id := request.ID
	version := 1
	if id == "" {
		id = uuid.New().String()
	} else {
		for _, policy := range policies {
			if policy.ID == id && policy.Version >= version {
				version = policy.Version + 1
			}
		}
	}
	policy := domain.FrequencyPolicy{
		ID: id, Version: version, Name: request.Name, Scope: request.Scope, ScopeRef: request.ScopeRef,
		Channel: request.Channel, MaxEvents: request.MaxEvents, WindowKind: request.WindowKind,
		WindowSeconds: request.WindowSeconds, Timezone: request.Timezone, DenyAction: request.DenyAction,
		Priority: request.Priority, Enabled: request.Enabled, CreatedAt: s.now().UTC(),
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if err := s.repository.SaveFrequencyPolicy(ctx, request.WorkspaceID, policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *FrequencyPolicyService) Evaluate(ctx context.Context, request FrequencyEvaluationRequest) (domain.FrequencyDecision, error) {
	policies, err := s.repository.ResolveFrequencyPolicies(ctx, request.WorkspaceID, request.CampaignRef, request.TriggerRef, request.Channel)
	if err != nil {
		return domain.FrequencyDecision{}, err
	}
	reservationID := request.EffectKey
	limiterPolicies := make([]FrequencyPolicy, 0, len(policies))
	policyIDs := make([]string, 0, len(policies))
	for _, policy := range policies {
		limiterPolicies = append(limiterPolicies, FrequencyPolicy{ID: policy.ID, Version: policy.Version, Scope: string(policy.Scope), MaxEvents: policy.MaxEvents, Window: time.Duration(policy.WindowSeconds) * time.Second})
		policyIDs = append(policyIDs, fmt.Sprintf("%s:v%d", policy.ID, policy.Version))
	}
	result, limiterErr := s.limiter.AllowAll(ctx, request.WorkspaceID, request.CustomerID, request.Channel, reservationID, limiterPolicies, request.OccurredAt)
	decision := domain.FrequencyDecision{ID: uuid.New().String(), ReservationID: reservationID, EffectKey: request.EffectKey,
		CustomerID: request.CustomerID, Channel: request.Channel, Allowed: result.Decision == FrequencyDecisionAllow,
		Deferred: result.Decision == FrequencyDecisionDefer, PolicyIDs: policyIDs, Reason: result.DeniedPolicyID, DecidedAt: request.OccurredAt.UTC()}
	if limiterErr != nil {
		decision.Reason = limiterErr.Error()
	}
	for _, policy := range policies {
		if fmt.Sprintf("%s:v%d", policy.ID, policy.Version) == result.DeniedPolicyID {
			decision.MatchedScope = policy.Scope
			break
		}
	}
	if err := s.repository.SaveFrequencyDecision(ctx, request.WorkspaceID, decision); err != nil {
		return domain.FrequencyDecision{}, err
	}
	return decision, limiterErr
}
