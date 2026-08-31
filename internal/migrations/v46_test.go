package migrations

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pgxTextArgumentCheckingExecutor struct {
	DBExecutor
}

func (e pgxTextArgumentCheckingExecutor) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "WITH customer_seeds AS") && len(args) > 0 {
		if _, err := pgtype.NewMap().Encode(pgtype.TextOID, pgtype.TextFormatCode, args[0], nil); err != nil {
			return nil, err
		}
	}
	return e.DBExecutor.ExecContext(ctx, query, args...)
}

func TestV46MigrationMetadataAndRegistration(t *testing.T) {
	migration := &V46Migration{}
	assert.Equal(t, 46.0, migration.GetMajorVersion())
	assert.True(t, migration.HasSystemUpdate())
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
	assert.Equal(t, "52.0", config.VERSION)
	registered, ok := GetRegisteredMigration(46.0)
	require.True(t, ok)
	assert.IsType(t, &V46Migration{}, registered)
}

func TestV46UpdateSystemAllocatesSequencesAndCopiesContactPermissions(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for _, statement := range schema.WorkspaceSequenceMigrationStatements() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("UPDATE user_workspaces.*jsonb_build_object.*customers.*contacts").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE workspace_invitations.*jsonb_build_object.*customers.*contacts").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = (&V46Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV46UpdateWorkspaceBackfillsCustomerProfileAndEncryptedIdentities(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for _, statement := range schema.CustomerTableDefinitions() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery("SELECT BTRIM.*external_id.*HAVING COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"external_id"}))
	mock.ExpectQuery("SELECT LOWER.*email.*HAVING COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"normalized_email"}))
	mock.ExpectExec("WITH customer_seeds AS.*INSERT INTO customers.*UPDATE contacts").
		WithArgs("42").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO customer_profiles").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO customer_tags").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO customer_list_memberships").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE contact_endpoints").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT c.customer_id, c.email, c.phone.*FROM contacts c").
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "email", "phone"}).
			AddRow("11111111-1111-4111-8111-111111111111", "Alice@Example.COM", "+86 138-0013-8000"))
	mock.ExpectExec("INSERT INTO customer_identities").
		WithArgs(sqlmock.AnyArg(), "11111111-1111-4111-8111-111111111111", "email", sqlmock.AnyArg(), sqlmock.AnyArg(), "a***@example.com", true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO customer_identities").
		WithArgs(sqlmock.AnyArg(), "11111111-1111-4111-8111-111111111111", "phone", sqlmock.AnyArg(), sqlmock.AnyArg(), "+86*******8000", true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = (&V46Migration{}).UpdateWorkspace(
		context.Background(),
		&config.Config{Security: config.SecurityConfig{SecretKey: "workspace-secret"}},
		&domain.Workspace{ID: "workspace-1", Sequence: 42},
		pgxTextArgumentCheckingExecutor{DBExecutor: db},
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV46UpdateWorkspaceRejectsInvalidSequenceOrMissingSecret(t *testing.T) {
	migration := &V46Migration{}

	err := migration.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1"}, nil)
	assert.ErrorContains(t, err, "workspace sequence")

	err = migration.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "workspace-1", Sequence: 1}, nil)
	assert.ErrorContains(t, err, "secret")
}

func TestInsertV46IdentityRejectsConflictInsteadOfSilentlyDroppingIdentity(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("INSERT INTO customer_identities").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = insertV46Identity(
		context.Background(), db, "workspace-secret", "workspace-1",
		"11111111-1111-4111-8111-111111111111", domain.CustomerIdentityPhone, "+8613800138000", true,
	)
	assert.ErrorContains(t, err, "already belongs to another customer")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBackfillV46CustomerIdentitiesReadsAllRowsBeforeInserting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	iterationErr := errors.New("driver: bad connection")
	mock.ExpectQuery("SELECT c.customer_id, c.email, c.phone.*FROM contacts c").
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "email", "phone"}).
			AddRow("11111111-1111-4111-8111-111111111111", "alice@example.com", nil).
			AddRow("22222222-2222-4222-8222-222222222222", "bob@example.com", nil).
			RowError(1, iterationErr))

	err = backfillV46CustomerIdentities(context.Background(), "workspace-secret", "workspace-1", db)
	assert.ErrorContains(t, err, "iterate legacy identities")
	assert.ErrorIs(t, err, iterationErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRejectV46DuplicateContactKeysNormalizesExternalIDsBeforeGrouping(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT BTRIM\(external_id\).*GROUP BY BTRIM\(external_id\)`).
		WillReturnRows(sqlmock.NewRows([]string{"external_id"}).AddRow("crm-42"))

	err = rejectV46DuplicateContactKeys(context.Background(), "workspace-1", db)
	assert.ErrorContains(t, err, `duplicate external user ID "crm-42"`)
	require.NoError(t, mock.ExpectationsWereMet())
}
