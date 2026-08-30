package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedMarketingPreflightSource struct {
	snapshot *domain.MarketingPreflightSnapshot
}

func (s fixedMarketingPreflightSource) LoadMarketingPreflightSnapshot(context.Context, string, string) (*domain.MarketingPreflightSnapshot, error) {
	copy := *s.snapshot
	return &copy, nil
}

func TestMarketingPreflightClassifiesBlockingAndWarnings(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	source := fixedMarketingPreflightSource{snapshot: &domain.MarketingPreflightSnapshot{
		WorkspaceID: "workspace-1", BroadcastID: "broadcast-1", BroadcastUpdatedAt: now,
		Counts:      domain.MarketingPreflightCounts{TargetTotal: 100, Reachable: 70, MissingIdentity: 5, MissingConsent: 10, Suppressed: 15, FrequencyDeny: 3, VariableFailures: 2},
		HasProvider: false, MissingTemplates: []string{"template-1"}, AudienceBuildStale: true,
	}}
	svc, err := NewMarketingPreflightService(source, nil, func() time.Time { return now })
	require.NoError(t, err)

	result, err := svc.PreflightBroadcast(context.Background(), domain.MarketingPreflightRequest{WorkspaceID: "workspace-1", BroadcastID: "broadcast-1"})
	require.NoError(t, err)
	assert.Equal(t, 2, result.BlockingCount)
	assert.GreaterOrEqual(t, result.WarningCount, 5)
	assert.Regexp(t, `^[a-f0-9]{64}\.[0-9]+$`, result.SummaryHash)
	assert.Equal(t, now.Add(5*time.Minute), result.ExpiresAt)
}

func TestMarketingPreflightValidationRejectsChangedSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	snapshot := &domain.MarketingPreflightSnapshot{
		WorkspaceID: "workspace-1", BroadcastID: "broadcast-1", BroadcastUpdatedAt: now,
		Counts: domain.MarketingPreflightCounts{TargetTotal: 10, Reachable: 10}, HasProvider: true,
	}
	svc, err := NewMarketingPreflightService(fixedMarketingPreflightSource{snapshot: snapshot}, nil, func() time.Time { return now })
	require.NoError(t, err)
	result, err := svc.PreflightBroadcast(context.Background(), domain.MarketingPreflightRequest{WorkspaceID: "workspace-1", BroadcastID: "broadcast-1"})
	require.NoError(t, err)
	require.NoError(t, svc.ValidateBroadcastPreflight(context.Background(), domain.MarketingPreflightRequest{WorkspaceID: "workspace-1", BroadcastID: "broadcast-1"}, result.SummaryHash))

	snapshot.Counts.Reachable = 9
	err = svc.ValidateBroadcastPreflight(context.Background(), domain.MarketingPreflightRequest{WorkspaceID: "workspace-1", BroadcastID: "broadcast-1"}, result.SummaryHash)
	assert.ErrorIs(t, err, domain.ErrMarketingPreflightChanged)
}

func TestMarketingPreflightValidationRequiresHashAndNoBlockers(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	snapshot := &domain.MarketingPreflightSnapshot{
		WorkspaceID: "workspace-1", BroadcastID: "broadcast-1", BroadcastUpdatedAt: now,
		Counts: domain.MarketingPreflightCounts{TargetTotal: 10}, HasProvider: false,
	}
	svc, err := NewMarketingPreflightService(fixedMarketingPreflightSource{snapshot: snapshot}, nil, func() time.Time { return now })
	require.NoError(t, err)
	request := domain.MarketingPreflightRequest{WorkspaceID: "workspace-1", BroadcastID: "broadcast-1"}
	assert.ErrorIs(t, svc.ValidateBroadcastPreflight(context.Background(), request, ""), domain.ErrMarketingPreflightRequired)
	result, err := svc.PreflightBroadcast(context.Background(), request)
	require.NoError(t, err)
	assert.ErrorIs(t, svc.ValidateBroadcastPreflight(context.Background(), request, result.SummaryHash), domain.ErrMarketingPreflightBlocked)
}
