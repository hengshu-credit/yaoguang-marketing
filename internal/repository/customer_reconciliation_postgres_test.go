package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerReconciliationRepairSQLIsBoundedAndNeverOverwritesConflicts(t *testing.T) {
	for _, reference := range customerReconciliationReferences {
		query := repairCustomerReferenceBatchSQL(reference)
		assert.Contains(t, query, "legacy.customer_id IS NULL")
		assert.Contains(t, query, "ORDER BY cursor_key")
		assert.Contains(t, query, "LIMIT $2")
		assert.Contains(t, query, "SET customer_id = batch.customer_id")
		assert.NotContains(t, query, "customer_id <> batch.customer_id")
	}
}

func TestCustomerReconciliationGetLoadsWorkspaceLocalRun(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-1").Return(db, nil)
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	runID := "11111111-1111-4111-8111-111111111111"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, job_type, status, batch_size, checkpoint, summary, last_error, started_at, updated_at, completed_at FROM customer_reconciliation_runs WHERE id = $1")).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "job_type", "status", "batch_size", "checkpoint", "summary", "last_error", "started_at", "updated_at", "completed_at"}).
			AddRow(runID, "scan", "completed", 2000, []byte(`{"contact_lists":"cursor-12"}`), []byte(`{"findings":[{"entity_name":"contact_lists","missing_count":2,"repairable_count":1}],"missing_count":2}`), nil, now, now, now))

	repo := NewCustomerReconciliationRepository(workspaceRepo)
	run, err := repo.Get(context.Background(), "workspace-1", runID)
	require.NoError(t, err)
	assert.Equal(t, "cursor-12", run.Checkpoint["contact_lists"])
	require.Len(t, run.Findings, 1)
	assert.Equal(t, int64(2), run.MissingCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
