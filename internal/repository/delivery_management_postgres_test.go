package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryProgressSeparatesQueuedAcceptedConfirmedAndUnknown(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repository := NewDeliveryRepositoryWithDB(db)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*FILTER").
		WithArgs(domain.DeliverySourceBroadcast, "broadcast-1", "version-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"total", "planned", "reserved", "queued", "submitting", "accepted", "confirmed",
			"suppressed", "deferred", "failed", "unknown", "cancelled",
		}).AddRow(12, 0, 1, 2, 1, 2, 3, 1, 0, 1, 1, 0))

	progress, err := repository.GetDeliveryProgress(context.Background(), "workspace-1", domain.DeliverySourceBroadcast, "broadcast-1", "version-1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), progress.Queued)
	assert.Equal(t, int64(2), progress.Accepted)
	assert.Equal(t, int64(3), progress.Confirmed)
	assert.Equal(t, int64(1), progress.Unknown)
	assert.Equal(t, int64(5), progress.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}
