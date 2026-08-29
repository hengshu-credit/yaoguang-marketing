package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCursorAtomicallyRotatesAndPersists(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO realtime_runtime_cursors").
		WithArgs("outbox-relay").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT last_workspace_id.*FOR UPDATE").
		WithArgs("outbox-relay").
		WillReturnRows(sqlmock.NewRows([]string{"last_workspace_id"}).AddRow("a"))
	mock.ExpectQuery("(?s)SELECT id.*FROM workspaces.*CASE WHEN id > \\$1").
		WithArgs("a", 3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("b").AddRow("c").AddRow("a"))
	mock.ExpectExec("UPDATE realtime_runtime_cursors").
		WithArgs("outbox-relay", "a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repository := NewWorkspaceCursorRepositoryWithDB(db)
	ids, err := repository.NextWorkspaceIDs(context.Background(), "outbox-relay", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c", "a"}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceCursorRejectsInvalidRequest(t *testing.T) {
	repository := NewWorkspaceCursorRepositoryWithDB(nil)

	_, err := repository.NextWorkspaceIDs(context.Background(), "", 1)
	require.Error(t, err)
	_, err = repository.NextWorkspaceIDs(context.Background(), "outbox-relay", 0)
	require.Error(t, err)
}
