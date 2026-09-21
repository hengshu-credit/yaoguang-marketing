package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"os"
	"regexp"
	"testing"
)

func TestConsoleThemeAtomicUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewConsoleThemeRepository(db)
	value := domain.DefaultConsoleTheme()
	query := regexp.QuoteMeta(`UPDATE workspaces SET settings = jsonb_set(COALESCE(settings,'{}'::jsonb), '{console_theme}', $1::jsonb, true), updated_at = CURRENT_TIMESTAMP WHERE id = $2`)
	mock.ExpectExec(query).WithArgs(sqlmock.AnyArg(), "workspace-a").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.Set(context.Background(), "workspace-a", &value))
	mock.ExpectExec(query).WithArgs(sqlmock.AnyArg(), "missing").WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, repo.Set(context.Background(), "missing", &value), sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConsoleThemePostgresPreservesConcurrentSettings(t *testing.T) {
	dsn := os.Getenv("CONSOLE_THEME_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL fixture required")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TEMP TABLE workspaces(id text primary key,name text,settings jsonb,integrations jsonb,updated_at timestamptz)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO workspaces VALUES('a','A','{"console_font":{"family":"Arial"},"website_url":"https://example.com"}','[]',now()),('b','B','{}','[]',now())`)
	require.NoError(t, err)
	value := domain.DefaultConsoleTheme()
	value.ColorScheme = "dark"
	require.NoError(t, NewConsoleThemeRepository(db).Set(context.Background(), "a", &value))
	var raw []byte
	require.NoError(t, db.QueryRow(`SELECT settings FROM workspaces WHERE id='a'`).Scan(&raw))
	var settings domain.WorkspaceSettings
	require.NoError(t, json.Unmarshal(raw, &settings))
	require.Equal(t, "Arial", settings.ConsoleFont.Family)
	// Simulate a form/worker that loaded settings before the theme write.
	stale := &domain.Workspace{ID: "a", Name: "Renamed", Settings: domain.WorkspaceSettings{Timezone: "UTC", SecretKey: "theme-test-secret", ConsoleFont: &domain.ConsoleFontSettings{Family: "Arial"}, WebsiteURL: "https://example.com"}}
	require.NoError(t, NewWorkspaceRepository(db, &config.DatabaseConfig{}, "theme-test-passphrase", nil).Update(context.Background(), stale))
	require.NoError(t, db.QueryRow(`SELECT settings FROM workspaces WHERE id='a'`).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &settings))
	require.Equal(t, "dark", settings.ConsoleTheme.ColorScheme)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM workspaces WHERE id='b' AND NOT settings ? 'console_theme'`).Scan(&count))
	require.Equal(t, 1, count)
}
