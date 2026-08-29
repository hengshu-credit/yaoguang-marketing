package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newContactEndpointRepositoryTest(t *testing.T) (*ContactEndpointPostgresRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	repo, err := NewContactEndpointRepositoryWithDB(db, "test-encryption-secret")
	require.NoError(t, err)
	return repo, mock, db
}

func TestContactEndpointRepositoryUpsertEncryptsAddress(t *testing.T) {
	repo, mock, db := newContactEndpointRepositoryTest(t)
	defer db.Close()
	endpoint := &domain.ContactEndpoint{
		EndpointID: "device-1", Channel: domain.ChannelPush,
		Provider: domain.PushProviderFCM, Platform: domain.EndpointPlatformAndroid,
		Address: "plain-device-token", Locale: "zh-CN", Timezone: "Asia/Shanghai",
		Enabled: true, Attributes: map[string]interface{}{"app_version": "1.2.3"},
	}
	mock.ExpectExec("INSERT INTO contact_endpoints").WithArgs(
		"device-1", "user@example.com", "push", "fcm", "android",
		sqlmock.AnyArg(), sqlmock.AnyArg(), "zh-CN", "Asia/Shanghai", "", "",
		sqlmock.AnyArg(), true,
	).WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Upsert(context.Background(), "workspace-1", "user@example.com", endpoint)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContactEndpointRepositoryDisableIsIdempotent(t *testing.T) {
	repo, mock, db := newContactEndpointRepositoryTest(t)
	defer db.Close()
	mock.ExpectExec("UPDATE contact_endpoints").
		WithArgs("device-1", "user@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Disable(context.Background(), "workspace-1", "user@example.com", "device-1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContactEndpointRepositoryListsActiveAndDecryptsInternally(t *testing.T) {
	repo, mock, db := newContactEndpointRepositoryTest(t)
	defer db.Close()
	ciphertext, err := repo.encryptAddress("plain-device-token")
	require.NoError(t, err)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"endpoint_id", "email", "channel", "provider", "platform", "address_ciphertext",
		"locale", "timezone", "app_id", "device_id", "attributes", "enabled", "version",
		"created_at", "updated_at", "last_seen_at",
	}).AddRow("device-1", "user@example.com", "push", "fcm", "android", ciphertext,
		"zh-CN", "Asia/Shanghai", nil, nil, []byte(`{"app_version":"1.2.3"}`), true, 2, now, now, now)
	mock.ExpectQuery("SELECT endpoint_id, email, channel").
		WithArgs("user@example.com", "push").WillReturnRows(rows)

	endpoints, err := repo.ListActiveByEmail(context.Background(), "workspace-1", "user@example.com", "push")
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	assert.Equal(t, "plain-device-token", endpoints[0].Address)
	encoded, err := endpoints[0].MarshalPublicJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "plain-device-token")
	require.NoError(t, mock.ExpectationsWereMet())
}
