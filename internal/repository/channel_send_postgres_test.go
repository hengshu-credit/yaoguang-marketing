package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelSendRepository(t *testing.T) (*ChannelSendPostgresRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").AnyTimes().Return(db, nil)
	repository, err := NewChannelSendRepository(workspaceRepo)
	require.NoError(t, err)
	return repository, mock, db
}

func channelSendRow(execution domain.ChannelSendExecution) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"effect_key", "request_hash", "message_id", "channel", "integration_id", "contact_email",
		"endpoint_id", "template_id", "template_version", "language", "status", "provider",
		"provider_message_id", "attempts", "last_error", "created_at", "updated_at",
	}).AddRow(
		execution.EffectKey, execution.RequestHash, execution.MessageID, execution.Channel,
		execution.IntegrationID, execution.ContactEmail, execution.EndpointID, execution.TemplateID,
		execution.TemplateVersion, nil, execution.Status, nil, nil, execution.Attempts, nil,
		execution.CreatedAt, execution.UpdatedAt,
	)
}

func TestChannelSendRepositoryReserveIsIdempotent(t *testing.T) {
	repository, mock, db := setupChannelSendRepository(t)
	defer db.Close()
	now := time.Now().UTC()
	execution := domain.ChannelSendExecution{
		EffectKey: "effect-1", RequestHash: "hash-1", MessageID: "message-1", Channel: domain.ChannelSMS,
		IntegrationID: "twilio-main", ContactEmail: "user@example.com", EndpointID: "phone-1",
		TemplateID: "ready", TemplateVersion: 2, Status: domain.ChannelSendReserved,
		CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectQuery("INSERT INTO channel_send_executions").WillReturnRows(channelSendRow(execution))
	stored, created, err := repository.Reserve(context.Background(), "ws-1", execution)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, execution.EffectKey, stored.EffectKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelSendRepositoryReserveRejectsHashConflict(t *testing.T) {
	repository, mock, db := setupChannelSendRepository(t)
	defer db.Close()
	execution := domain.ChannelSendExecution{
		EffectKey: "effect-1", RequestHash: "new-hash", MessageID: "message-1", Channel: domain.ChannelPush,
		IntegrationID: "fcm-main", ContactEmail: "user@example.com", EndpointID: "device-1",
		TemplateID: "ready", TemplateVersion: 1, Status: domain.ChannelSendReserved,
	}
	mock.ExpectQuery("INSERT INTO channel_send_executions").WillReturnError(sql.ErrNoRows)
	existing := execution
	existing.RequestHash = "old-hash"
	existing.CreatedAt, existing.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	mock.ExpectQuery("SELECT .* FROM channel_send_executions WHERE effect_key").WillReturnRows(channelSendRow(existing))
	_, _, err := repository.Reserve(context.Background(), "ws-1", execution)
	assert.ErrorIs(t, err, domain.ErrChannelSendHashConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelSendRepositoryConfirmWritesHistoryAtomically(t *testing.T) {
	repository, mock, db := setupChannelSendRepository(t)
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE channel_send_executions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO message_history").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	message := &domain.MessageHistory{
		ID: "message-1", ContactEmail: "user@example.com", TemplateID: "ready", TemplateVersion: 2,
		Channel: domain.ChannelSMS, SentAt: now,
		MessageData: domain.MessageData{Data: map[string]interface{}{"private": "value"}, Metadata: map[string]interface{}{"effect_key": "effect-1"}},
	}
	confirmed, err := repository.Confirm(context.Background(), "ws-1", "effect-1", "twilio", "SM123", "secret", message, now)
	require.NoError(t, err)
	assert.True(t, confirmed)
	require.NoError(t, mock.ExpectationsWereMet())
}
