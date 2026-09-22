package migrations

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestV58ExpandsImportJobStatuses(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE import_jobs DROP CONSTRAINT IF EXISTS import_jobs_status_check`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE import_jobs ADD CONSTRAINT import_jobs_status_check CHECK (status IN ('uploading', 'staged', 'processing', 'completed', 'needs_attention', 'retrying', 'rejected', 'cancelled'))`)).WillReturnResult(sqlmock.NewResult(0, 0))

	migration := &V58Migration{}
	require.NoError(t, migration.UpdateWorkspace(context.Background(), nil, nil, db))
	require.NoError(t, mock.ExpectationsWereMet())
	registered, ok := GetRegisteredMigration(58)
	require.True(t, ok)
	require.IsType(t, migration, registered)
	require.Error(t, migration.UpdateWorkspace(context.Background(), nil, nil, nil))
}

func TestV58PropagatesConstraintFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	failure := errors.New("DDL unavailable")
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE import_jobs DROP CONSTRAINT IF EXISTS import_jobs_status_check`)).WillReturnError(failure)
	require.ErrorIs(t, (&V58Migration{}).UpdateWorkspace(context.Background(), nil, nil, db), failure)
	require.NoError(t, mock.ExpectationsWereMet())
}
