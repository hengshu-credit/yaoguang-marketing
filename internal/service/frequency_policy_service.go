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
}

func NewFrequencyPolicyService(repository domain.FrequencyPolicyRepository, limiter *FrequencyLimiter) (*FrequencyPolicyService, error) {
	if repository == nil || limiter == nil {
		return nil, fmt.Errorf("frequency policy repository and limiter are required")
	}
	return &FrequencyPolicyService{repository: repository, limiter: limiter}, nil
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
