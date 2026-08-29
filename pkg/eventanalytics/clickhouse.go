package eventanalytics

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const EventProjectionTableDDL = `CREATE TABLE IF NOT EXISTS %s.event_projection
(
    workspace_id String,
    event_id UUID,
    event_type LowCardinality(String),
    schema_version UInt16,
    subject_type LowCardinality(String),
    subject_id String,
    contact_email Nullable(String),
    source LowCardinality(String),
    correlation_id UUID,
    causation_id Nullable(UUID),
    occurred_at DateTime64(3, 'UTC'),
    received_at DateTime64(3, 'UTC'),
    projected_at DateTime64(3, 'UTC'),
    payload_json String,
    envelope_json String
)
ENGINE = ReplacingMergeTree(projected_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (workspace_id, event_type, occurred_at, event_id)`

const EventProjectionLogicalQuery = `
SELECT workspace_id, event_id, event_type, schema_version, subject_type,
       subject_id, contact_email, source, correlation_id, causation_id,
       occurred_at, received_at, projected_at, payload_json, envelope_json
FROM %s.event_projection FINAL`

var clickHouseIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type ClickHouseStore struct {
	connection clickhouse.Conn
	database   string
	now        func() time.Time
}

func NewClickHouseStore(connection clickhouse.Conn, database string) (*ClickHouseStore, error) {
	if connection == nil {
		return nil, errors.New("clickhouse connection is required")
	}
	if !clickHouseIdentifier.MatchString(database) {
		return nil, errors.New("clickhouse database name is invalid")
	}
	return &ClickHouseStore{
		connection: connection, database: database,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func OpenClickHouseStore(options *clickhouse.Options, database string) (*ClickHouseStore, error) {
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	store, err := NewClickHouseStore(connection, database)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return store, nil
}

func (s *ClickHouseStore) Close() error {
	return s.connection.Close()
}

func (s *ClickHouseStore) Ping(ctx context.Context) error {
	return s.connection.Ping(ctx)
}

func (s *ClickHouseStore) EnsureSchema(ctx context.Context) error {
	if err := s.connection.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.database)); err != nil {
		return fmt.Errorf("create clickhouse database: %w", err)
	}
	if err := s.connection.Exec(ctx, fmt.Sprintf(EventProjectionTableDDL, s.database)); err != nil {
		return fmt.Errorf("create clickhouse event projection: %w", err)
	}
	// Non-destructive compatibility for volumes initialized by the first
	// realtime Compose draft. Fresh tables already contain these columns.
	columns := []string{
		"subject_type LowCardinality(String)",
		"subject_id String",
		"contact_email Nullable(String)",
		"correlation_id UUID",
		"causation_id Nullable(UUID)",
		"received_at DateTime64(3, 'UTC')",
		"projected_at DateTime64(3, 'UTC') DEFAULT now64(3)",
		"envelope_json String",
	}
	for _, column := range columns {
		if err := s.connection.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE %s.event_projection ADD COLUMN IF NOT EXISTS %s", s.database, column,
		)); err != nil {
			return fmt.Errorf("upgrade clickhouse event projection column %s: %w", column, err)
		}
	}
	return nil
}

func (s *ClickHouseStore) InsertBatch(ctx context.Context, events []domain.EventEnvelope) error {
	if len(events) == 0 {
		return nil
	}
	projectedAt := s.now().UTC()
	batch, err := s.connection.PrepareBatch(ctx, fmt.Sprintf(`
		INSERT INTO %s.event_projection (
			workspace_id, event_id, event_type, schema_version,
			subject_type, subject_id, contact_email, source,
			correlation_id, causation_id, occurred_at, received_at,
			projected_at, payload_json, envelope_json
		)
	`, s.database))
	if err != nil {
		return fmt.Errorf("prepare clickhouse event batch: %w", err)
	}
	for _, event := range events {
		projection, projectionErr := NewEventProjection(event, projectedAt)
		if projectionErr != nil {
			_ = batch.Abort()
			return projectionErr
		}
		if err := batch.Append(
			projection.WorkspaceID, projection.EventID, projection.EventType,
			uint16(projection.SchemaVersion), projection.SubjectType,
			projection.SubjectID, nullableString(projection.ContactEmail), projection.Source,
			projection.CorrelationID, projection.CausationID,
			projection.OccurredAt, projection.ReceivedAt, projection.ProjectedAt,
			projection.PayloadJSON, projection.EnvelopeJSON,
		); err != nil {
			_ = batch.Abort()
			return fmt.Errorf("append clickhouse event projection: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send clickhouse event batch: %w", err)
	}
	return nil
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
