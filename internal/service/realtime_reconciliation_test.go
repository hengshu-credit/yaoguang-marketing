package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

type fixedMatchAuditRepository struct {
	summary domain.MatchReconciliationSummary
	err     error
}

func (r fixedMatchAuditRepository) SummarizeMatchAudits(context.Context, string, time.Time, time.Time) (domain.MatchReconciliationSummary, error) {
	return r.summary, r.err
}

func TestRealtimeReconciliationRequiresFullShadowWindow(t *testing.T) {
	service, err := NewRealtimeReconciliationService(fixedMatchAuditRepository{}, ShadowCutoverPolicy{
		MinimumWindow: 24 * time.Hour, MinimumConsistencyRate: 0.9999,
	})
	require.NoError(t, err)
	to := time.Now().UTC()

	assessment, err := service.AssessPrimaryCutover(context.Background(), "workspace-1", to.Add(-23*time.Hour), to)

	require.NoError(t, err)
	assert.False(t, assessment.Ready)
	assert.Contains(t, assessment.Blockers, "shadow window is shorter than 24h0m0s")
}

func TestRealtimeReconciliationBlocksUnexplainedMissingAndMismatch(t *testing.T) {
	repo := fixedMatchAuditRepository{summary: domain.MatchReconciliationSummary{
		RealtimeEvaluated: 100_000, Agreements: 99_990, DecisionMismatches: 10,
		MissingRealtime: 1, ConsistencyRate: 0.9999,
	}}
	service, err := NewRealtimeReconciliationService(repo, DefaultShadowCutoverPolicy())
	require.NoError(t, err)
	to := time.Now().UTC()

	assessment, err := service.AssessPrimaryCutover(context.Background(), "workspace-1", to.Add(-25*time.Hour), to)

	require.NoError(t, err)
	assert.False(t, assessment.Ready)
	assert.Contains(t, assessment.Blockers, "legacy matches missing realtime decisions: 1")
}

func TestRealtimeReconciliationAllowsHealthyShadow(t *testing.T) {
	repo := fixedMatchAuditRepository{summary: domain.MatchReconciliationSummary{
		RealtimeEvaluated: 1_000_000, Agreements: 999_950, DecisionMismatches: 50,
		ConsistencyRate: 0.99995,
	}}
	service, err := NewRealtimeReconciliationService(repo, DefaultShadowCutoverPolicy())
	require.NoError(t, err)
	to := time.Now().UTC()

	assessment, err := service.AssessPrimaryCutover(context.Background(), "workspace-1", to.Add(-25*time.Hour), to)

	require.NoError(t, err)
	assert.True(t, assessment.Ready)
	assert.Empty(t, assessment.Blockers)
}
