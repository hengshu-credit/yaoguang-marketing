package migrations

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV51MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V51Migration{}
	assert.Equal(t, 51.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "54.0", config.VERSION)
	registered, ok := GetRegisteredMigration(51.0)
	require.True(t, ok)
	assert.IsType(t, &V51Migration{}, registered)
}

func TestV51UpdateWorkspaceRegeneratesEveryCustomerNumberAndResolvesSuffixCollisions(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Date(2026, time.August, 30, 7, 30, 45, 0, time.UTC)
	firstID := "00000000-0000-0000-0000-000000000001"
	// This UUID differs by exactly 36^6, so its primary six-character suffix
	// collides with firstID and exercises deterministic collision allocation.
	secondID := "00000000-0000-0000-0000-000081bf1001"
	mock.ExpectExec(regexp.QuoteMeta(v51CreateMappingTableSQL)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(v51SelectFirstCustomerPageSQL)).
		WithArgs(v51CustomerPageSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(firstID, createdAt).
			AddRow(secondID, createdAt))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO yaoguang_v51_customer_numbers (customer_id, customer_no) VALUES ($1, $2), ($3, $4)")).
		WithArgs(
			firstID, "U0272026083015304508000001",
			secondID, "U0272026083015304508000002",
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(v51AssignTemporaryNumbersSQL)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(v51ApplyCustomerNumbersSQL)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(v51RefreshIdempotencyResponsesSQL)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err = (&V51Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{},
		&domain.Workspace{ID: "workspace-1", Sequence: 27},
		db,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStageV51CustomerNumbersPagesWithoutLosingCollisionState(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Date(2026, time.August, 30, 7, 30, 45, 0, time.UTC)
	firstID := "00000000-0000-0000-0000-000000000001"
	secondID := "00000000-0000-0000-0000-000081bf1001"
	thirdID := "00000000-0000-0000-0000-0001037e2001"
	mock.ExpectQuery(regexp.QuoteMeta(v51SelectFirstCustomerPageSQL)).WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(firstID, createdAt).
			AddRow(secondID, createdAt))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO yaoguang_v51_customer_numbers (customer_id, customer_no) VALUES ($1, $2), ($3, $4)")).
		WithArgs(firstID, "U0272026083015304508000001", secondID, "U0272026083015304508000002").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(regexp.QuoteMeta(v51SelectNextCustomerPageSQL)).WithArgs(createdAt, secondID, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(thirdID, createdAt))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO yaoguang_v51_customer_numbers (customer_id, customer_no) VALUES ($1, $2)")).
		WithArgs(thirdID, "U0272026083015304508000003").
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := stageV51CustomerNumbers(context.Background(), 27, db, 2)

	require.NoError(t, err)
	assert.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV51UpdateWorkspaceRejectsInvalidWorkspaceBeforeReadingCustomers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	err = (&V51Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, db)

	require.Error(t, err)
	assert.ErrorContains(t, err, "workspace sequence")
	require.NoError(t, mock.ExpectationsWereMet())
}
