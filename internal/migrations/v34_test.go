package migrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV34Migration_GetMajorVersion(t *testing.T) {
	assert.Equal(t, 34.0, (&V34Migration{}).GetMajorVersion())
}

func TestV34Migration_HasSystemUpdate(t *testing.T) {
	assert.True(t, (&V34Migration{}).HasSystemUpdate())
}

func TestV34Migration_HasWorkspaceUpdate(t *testing.T) {
	assert.False(t, (&V34Migration{}).HasWorkspaceUpdate())
}

func TestV34Migration_ShouldRestartServer(t *testing.T) {
	assert.False(t, (&V34Migration{}).ShouldRestartServer())
}

func TestV34Migration_UpdateSystem_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS federated_identities`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_federated_identities_user_id`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS idx_users_lower_email`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`federated_identities_user_id_fkey`).WillReturnResult(sqlmock.NewResult(0, 0))

	err = (&V34Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestV34Migration_UpdateSystem_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS federated_identities`).WillReturnError(assert.AnError)

	err = (&V34Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v34 system migration failed")
}

func TestV34Migration_UpdateWorkspace_NoOp(t *testing.T) {
	assert.NoError(t, (&V34Migration{}).UpdateWorkspace(context.Background(), &config.Config{},
		&domain.Workspace{ID: "ws_test"}, nil))
}

func TestV34Migration_Registered(t *testing.T) {
	for _, m := range GetRegisteredMigrations() {
		if m.GetMajorVersion() == 34.0 {
			return
		}
	}
	t.Fatal("V34Migration not registered")
}
