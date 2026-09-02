package service

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
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

type currentAudienceRecipientCounterStub struct {
	counts      domain.MarketingPreflightCounts
	workspaceID string
	audienceID  string
	channel     string
}

func (s *currentAudienceRecipientCounterStub) CountCurrentAudienceRecipients(_ context.Context, workspaceID, audienceID, channel string) (domain.MarketingPreflightCounts, error) {
	s.workspaceID = workspaceID
	s.audienceID = audienceID
	s.channel = channel
	return s.counts, nil
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

func TestMarketingPreflightDoesNotTreatConsentAsRecipientEligibility(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	source := fixedMarketingPreflightSource{snapshot: &domain.MarketingPreflightSnapshot{
		WorkspaceID: "workspace-1", BroadcastID: "broadcast-1", BroadcastUpdatedAt: now,
		Counts:             domain.MarketingPreflightCounts{TargetTotal: 10, MissingConsent: 10},
		HasProvider:        true,
		HasFrequencyPolicy: true,
	}}
	svc, err := NewMarketingPreflightService(source, nil, func() time.Time { return now })
	require.NoError(t, err)

	result, err := svc.PreflightBroadcast(context.Background(), domain.MarketingPreflightRequest{
		WorkspaceID: "workspace-1", BroadcastID: "broadcast-1",
	})
	require.NoError(t, err)
	assert.Zero(t, result.Counts.MissingConsent)
	assert.NotContains(t, result.Issues, domain.MarketingPreflightIssue{Code: "consent_missing"})
	for _, issue := range result.Issues {
		assert.NotEqual(t, "consent_missing", issue.Code)
	}
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

func TestBroadcastMarketingPreflightUsesCurrentDefinitionForFloatingAudience(t *testing.T) {
	controller := gomock.NewController(t)
	broadcasts := mocks.NewMockBroadcastRepository(controller)
	workspaces := mocks.NewMockWorkspaceRepository(controller)
	templates := mocks.NewMockTemplateService(controller)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	updatedAt := time.Date(2026, 9, 2, 2, 3, 4, 0, time.UTC)
	broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(&domain.Broadcast{
		ID:          "broadcast-1",
		WorkspaceID: "workspace-1",
		ChannelType: "email",
		Audience: domain.AudienceSettings{
			AudienceID: "11111111-1111-4111-8111-111111111111",
		},
		TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}}},
		UpdatedAt:    updatedAt,
	}, nil)
	workspaces.EXPECT().GetByID(gomock.Any(), "workspace-1").Return(&domain.Workspace{ID: "workspace-1"}, nil)
	templates.EXPECT().GetTemplateByID(gomock.Any(), "workspace-1", "template-1", int64(0)).Return(&domain.Template{
		ID: "template-1", Channel: "email",
	}, nil)
	workspaces.EXPECT().GetConnection(gomock.Any(), "workspace-1").Return(db, nil)
	sqlMock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("broadcast-1", "email").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	counter := &currentAudienceRecipientCounterStub{counts: domain.MarketingPreflightCounts{
		TargetTotal: 1, Reachable: 1,
	}}

	source, err := NewBroadcastMarketingPreflightSource(broadcasts, workspaces, templates, counter)
	require.NoError(t, err)
	snapshot, err := source.LoadMarketingPreflightSnapshot(context.Background(), "workspace-1", "broadcast-1")
	require.NoError(t, err)
	assert.Equal(t, domain.MarketingPreflightCounts{TargetTotal: 1, Reachable: 1}, snapshot.Counts)
	assert.False(t, snapshot.AudienceBuildStale)
	assert.Equal(t, "workspace-1", counter.workspaceID)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", counter.audienceID)
	assert.Equal(t, "email", counter.channel)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestLoadMarketingRecipientCountsDoesNotRequireConsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`(?s)WITH classified AS \(.*customer_identities identity.*COUNT\(\*\) FILTER \(WHERE has_identity AND NOT suppressed\).*COUNT\(\*\) FILTER \(WHERE NOT has_identity\),\s*0::bigint`).
		WithArgs("list-1", "email").
		WillReturnRows(sqlmock.NewRows([]string{"target_total", "reachable", "missing_identity", "missing_consent", "suppressed"}).
			AddRow(int64(1), int64(1), int64(0), int64(0), int64(0)))

	counts := domain.MarketingPreflightCounts{}
	err = loadMarketingRecipientCounts(context.Background(), db, domain.AudienceSettings{List: "list-1"}, "email", &counts)
	require.NoError(t, err)
	assert.Equal(t, domain.MarketingPreflightCounts{TargetTotal: 1, Reachable: 1}, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}
