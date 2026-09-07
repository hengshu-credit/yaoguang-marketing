package migrations

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/stretchr/testify/require"
)

func TestV57InstallsConfigurableMarketing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, query := range schema.ConfigurableMarketingTableDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	require.NoError(t, (&V57Migration{}).UpdateWorkspace(context.Background(), nil, nil, db))
	require.NoError(t, mock.ExpectationsWereMet())
	migration, ok := GetRegisteredMigration(57)
	require.True(t, ok)
	require.IsType(t, &V57Migration{}, migration)
	require.Error(t, (&V57Migration{}).UpdateWorkspace(context.Background(), nil, nil, nil))
}

func TestV57PropagatesInstallationFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	failure := errors.New("DDL unavailable")
	mock.ExpectExec(regexp.QuoteMeta(schema.ConfigurableMarketingTableDefinitions()[0])).WillReturnError(failure)
	require.ErrorIs(t, (&V57Migration{}).UpdateWorkspace(context.Background(), nil, nil, db), failure)
	require.NoError(t, mock.ExpectationsWereMet())
}
