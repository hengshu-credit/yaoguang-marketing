CREATE DATABASE IF NOT EXISTS notifuse;

CREATE TABLE IF NOT EXISTS notifuse.event_projection
(
    workspace_id String,
    event_id UUID,
    event_type LowCardinality(String),
    schema_version UInt16,
    occurred_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
    contact_id Nullable(String),
    source LowCardinality(String),
    payload_json String
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (workspace_id, event_type, occurred_at, event_id);

CREATE TABLE IF NOT EXISTS notifuse.delivery_projection
(
    workspace_id String,
    effect_key String,
    event_id UUID,
    channel LowCardinality(String),
    status LowCardinality(String),
    provider Nullable(String),
    occurred_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
    metadata_json String
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (workspace_id, channel, occurred_at, effect_key);
