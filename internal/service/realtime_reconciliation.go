package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type ShadowCutoverPolicy struct {
	MinimumWindow          time.Duration
	MinimumConsistencyRate float64
	MinimumEvaluated       int64
}

func DefaultShadowCutoverPolicy() ShadowCutoverPolicy {
	return ShadowCutoverPolicy{
		MinimumWindow: 24 * time.Hour, MinimumConsistencyRate: 0.9999, MinimumEvaluated: 1,
	}
}

type PrimaryCutoverAssessment = domain.PrimaryCutoverAssessment

type RealtimeReconciliationService struct {
	repository domain.MatchAuditRepository
	policy     ShadowCutoverPolicy
}

func NewRealtimeReconciliationService(
	repository domain.MatchAuditRepository,
	policy ShadowCutoverPolicy,
) (*RealtimeReconciliationService, error) {
	if repository == nil {
		return nil, errors.New("match audit repository is required")
	}
	if policy.MinimumWindow <= 0 {
		return nil, errors.New("minimum shadow window must be positive")
	}
	if policy.MinimumConsistencyRate <= 0 || policy.MinimumConsistencyRate > 1 {
		return nil, errors.New("minimum consistency rate must be in (0, 1]")
	}
	if policy.MinimumEvaluated < 0 {
		return nil, errors.New("minimum evaluated count cannot be negative")
	}
	return &RealtimeReconciliationService{repository: repository, policy: policy}, nil
}

func (s *RealtimeReconciliationService) AssessPrimaryCutover(
	ctx context.Context,
	workspaceID string,
	from time.Time,
	to time.Time,
) (PrimaryCutoverAssessment, error) {
	if workspaceID == "" {
		return PrimaryCutoverAssessment{}, errors.New("workspace id is required")
	}
	from, to = from.UTC(), to.UTC()
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return PrimaryCutoverAssessment{}, errors.New("valid reconciliation time window is required")
	}
	summary, err := s.repository.SummarizeMatchAudits(ctx, workspaceID, from, to)
	if err != nil {
		return PrimaryCutoverAssessment{}, fmt.Errorf("summarize shadow decisions: %w", err)
	}
	summary.WorkspaceID, summary.From, summary.To = workspaceID, from, to
	if summary.RealtimeEvaluated > 0 {
		summary.ConsistencyRate = float64(summary.Agreements) / float64(summary.RealtimeEvaluated)
	}
	assessment := PrimaryCutoverAssessment{Summary: summary}
	if to.Sub(from) < s.policy.MinimumWindow {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf(
			"shadow window is shorter than %s", s.policy.MinimumWindow,
		))
	}
	if summary.RealtimeEvaluated < s.policy.MinimumEvaluated {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf(
			"realtime decisions %d are below minimum %d", summary.RealtimeEvaluated, s.policy.MinimumEvaluated,
		))
	}
	if summary.ConsistencyRate < s.policy.MinimumConsistencyRate {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf(
			"decision consistency %.6f is below %.6f", summary.ConsistencyRate, s.policy.MinimumConsistencyRate,
		))
	}
	if summary.MissingRealtime > 0 {
		assessment.Blockers = append(assessment.Blockers, fmt.Sprintf(
			"legacy matches missing realtime decisions: %d", summary.MissingRealtime,
		))
	}
	assessment.Ready = len(assessment.Blockers) == 0
	return assessment, nil
}
