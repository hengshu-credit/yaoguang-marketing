package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/eventanalytics"
)

func TestRealtimeClickHouseProjection(t *testing.T) {
	address := os.Getenv("TEST_CLICKHOUSE_ADDR")
	if address == "" {
		t.Skip("TEST_CLICKHOUSE_ADDR is not configured")
	}
	database := os.Getenv("TEST_CLICKHOUSE_DATABASE")
	if database == "" {
		database = "notifuse"
	}
	connection, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{address},
		Auth: clickhouse.Auth{
			Database: database,
			Username: os.Getenv("TEST_CLICKHOUSE_USER"),
			Password: os.Getenv("TEST_CLICKHOUSE_PASSWORD"),
		},
	})
	require.NoError(t, err)
	store, err := eventanalytics.NewClickHouseStore(connection, database)
	require.NoError(t, err)
	defer store.Close()
	ctx := context.Background()
	require.NoError(t, store.Ping(ctx))
	require.NoError(t, store.EnsureSchema(ctx))

	workspaceID := "integration-" + uuid.NewString()
	defer func() {
		_ = connection.Exec(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s.event_projection DELETE WHERE workspace_id = ?", database,
		), workspaceID)
	}()
	eventID := uuid.New()
	now := time.Now().UTC()
	event := domain.EventEnvelope{
		ID: uuid.New(), EventID: eventID, Type: "contact.updated", SchemaVersion: 1,
		WorkspaceID: workspaceID,
		Subject:     domain.EventSubject{Type: "contact", ID: "contact-1", ContactEmail: "person@example.com"},
		Source:      "integration-test", OccurredAt: now, ReceivedAt: now,
		CorrelationID: uuid.New(), Data: json.RawMessage(`{"changes":{"language":{"new":"fr"}}}`),
	}
	require.NoError(t, store.InsertBatch(ctx, []domain.EventEnvelope{event, event}))

	var count uint64
	err = connection.QueryRow(ctx, fmt.Sprintf(`
		SELECT count() FROM %s.event_projection FINAL
		WHERE workspace_id = ? AND event_id = ?
	`, database), workspaceID, eventID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)

}
