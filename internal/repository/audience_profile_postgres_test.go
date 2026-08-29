package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAudienceProfileRepositoryTest(t *testing.T) (*AudienceProfilePostgresRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewAudienceProfileRepositoryWithDB(db), mock, db
}

func TestAudienceProfileRepositoryGetProfilesUsesOneSetBasedQuery(t *testing.T) {
	repo, mock, db := newAudienceProfileRepositoryTest(t)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"email", "status", "attributes", "tags"}).
		AddRow("a@example.com", "active", []byte(`{"plan":"pro"}`), "{beta,paid}").
		AddRow("b@example.com", nil, []byte(`{}`), "{}")
	mock.ExpectQuery("FROM unnest\\(\\$1::text\\[\\]\\) AS requested\\(email\\)").
		WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)

	profiles, err := repo.GetProfiles(context.Background(), "workspace-1", []string{"a@example.com", "b@example.com"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "active", *profiles["a@example.com"].Status)
	assert.Equal(t, "pro", profiles["a@example.com"].Attributes["plan"])
	assert.Equal(t, []string{"beta", "paid"}, profiles["a@example.com"].Tags)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAudienceProfileRepositoryEnsureContactsUsesSetBasedInsert(t *testing.T) {
	repo, mock, db := newAudienceProfileRepositoryTest(t)
	defer db.Close()
	mock.ExpectExec("INSERT INTO contacts \\(email\\).*unnest").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := repo.EnsureContacts(context.Background(), "workspace-1", []string{"a@example.com", "b@example.com"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAudienceProfileRepositoryUpsertProfileMergesAttributes(t *testing.T) {
	repo, mock, db := newAudienceProfileRepositoryTest(t)
	defer db.Close()
	status := "active"
	mock.ExpectExec("INSERT INTO contact_profiles").
		WithArgs("user@example.com", &status, sqlmock.AnyArg(), true).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertProfile(context.Background(), "workspace-1", "user@example.com", &status, map[string]interface{}{"plan": "pro"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAudienceProfileRepositoryApplyTagsSetIsTransactionalAndSorted(t *testing.T) {
	repo, mock, db := newAudienceProfileRepositoryTest(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM contact_tags").
		WithArgs("user@example.com", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO contact_tags").
		WithArgs("user@example.com", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	rows := sqlmock.NewRows([]string{"tag"}).AddRow("beta").AddRow("paid")
	mock.ExpectQuery("SELECT tag FROM contact_tags").
		WithArgs("user@example.com").WillReturnRows(rows)
	mock.ExpectCommit()

	tags, err := repo.ApplyTags(context.Background(), "workspace-1", "user@example.com", domain.TagOperationSet, []string{"paid", "beta"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta", "paid"}, tags)
	require.NoError(t, mock.ExpectationsWereMet())
}
