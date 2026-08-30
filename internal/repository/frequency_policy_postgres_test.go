package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrequencyPolicyRepositoryResolvesAllThreeScopes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "version", "name", "scope", "scope_ref", "channel", "max_events", "window_kind", "window_seconds", "timezone", "deny_action", "priority", "enabled", "created_at"}).
		AddRow("11111111-1111-4111-8111-111111111111", 1, "campaign", "campaign", "campaign-1", "email", 1, "sliding", 3600, "", "suppress", 10, true, now).
		AddRow("22222222-2222-4222-8222-222222222222", 2, "trigger", "trigger", "automation-1:event", "email", 2, "sliding", 86400, "", "suppress", 20, true, now).
		AddRow("33333333-3333-4333-8333-333333333333", 3, "global", "workspace_global", "", "email", 3, "sliding", 604800, "", "defer", 30, true, now)
	mock.ExpectQuery("(?s)SELECT id.*SELECT DISTINCT ON.*frequency_policies").WithArgs("email", "campaign-1", "automation-1:event").WillReturnRows(rows)
	repo := NewFrequencyPolicyRepositoryWithDB(db)
	policies, err := repo.ResolveFrequencyPolicies(context.Background(), "workspace-1", "campaign-1", "automation-1:event", "email")
	require.NoError(t, err)
	require.Len(t, policies, 3)
	assert.Equal(t, domain.FrequencyScopeCampaign, policies[0].Scope)
	assert.Equal(t, domain.FrequencyScopeWorkspaceGlobal, policies[2].Scope)
	require.NoError(t, mock.ExpectationsWereMet())
}
