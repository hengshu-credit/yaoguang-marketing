package migrations

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/stretchr/testify/require"
)

func TestV56InstallsSharedActivityTriggers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, query := range schema.CustomerActivityTimelineDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	require.NoError(t, (&V56Migration{}).UpdateWorkspace(context.Background(), nil, nil, db))
	require.NoError(t, mock.ExpectationsWereMet())
	m, ok := GetRegisteredMigration(56)
	require.True(t, ok)
	require.IsType(t, &V56Migration{}, m)
	require.Error(t, (&V56Migration{}).UpdateWorkspace(context.Background(), nil, nil, nil))
}
