# High-Concurrency Realtime Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Notifuse's per-automation trigger and single-process polling bottlenecks with a horizontally scalable, recoverable realtime runtime on PostgreSQL/PgBouncer, RabbitMQ, Redis, ClickHouse, and MinIO.

**Architecture:** PostgreSQL remains the only online source of truth. A transactional outbox publishes durable envelopes to RabbitMQ; indexed rule, journey, delivery, and analytics workers consume idempotently. Redis is an optimization and frequency-cap dependency, ClickHouse is a rebuildable projection, and MinIO stores binaries only. `legacy`, `shadow`, and `primary` modes support exact-event comparison and rollback.

**Tech Stack:** Go 1.25.4, PostgreSQL 17, PgBouncer 1.23.1, RabbitMQ 4, Redis 7, ClickHouse 24.9, MinIO/S3, Docker Compose, `lib/pq`, `amqp091-go`, `go-redis/v9`, `clickhouse-go/v2`, `minio-go/v7`.

**Spec:** `docs/superpowers/specs/2026-08-28-high-concurrency-runtime-design.md`

## Global Constraints

- Work directly on the existing `dev` branch; do not create a branch or worktree.
- Preserve the system-database plus per-workspace-database tenancy model.
- Preserve existing RPC API routes, scoped API keys, permissions, managed Webhooks, email delivery, and console behavior.
- PostgreSQL 17 is the only online source of truth; RabbitMQ, Redis, and ClickHouse must never become authoritative.
- `REALTIME_MODE` values are exactly `legacy`, `shadow`, and `primary`; default is `legacy`.
- `NOTIFUSE_ROLE` values are exactly `all`, `api`, `outbox-relay`, `rule-worker`, `journey-worker`, `delivery-worker`, `analytics-worker`, and `scheduler`; default is `all`.
- Every new or changed Go behavior follows RED-GREEN-REFACTOR. Configuration-only changes receive executable structural tests before the configuration is edited.
- Every workspace schema change is migration v40, idempotent, transactional, tested from v39, reflected in fresh-workspace initialization, and documented in `CHANGELOG.md`.
- Altering `contact_timeline` requires regeneration and validation of all live automation trigger functions during migration.
- RabbitMQ publishing uses confirms; consumers use manual ACK and PostgreSQL inbox idempotency.
- Redis failure may reduce performance but cannot corrupt rules or journey state; an unavailable frequency limiter must defer sending.
- ClickHouse deletion or outage cannot block event acceptance, journey enrollment, or delivery.
- Infrastructure credentials come from environment variables and Compose defaults are explicitly development-only.
- Verification commands run inside the Go 1.25 Docker image because the host has no Go toolchain.

---

### Task 1: Runtime roles and realtime configuration

**Files:**
- Create: `config/realtime.go`
- Create: `config/realtime_test.go`
- Modify: `config/config.go`
- Modify: `config/config_test.go`
- Create: `internal/app/role.go`
- Create: `internal/app/role_test.go`

**Interfaces:**
- Produces: `config.RuntimeRole`, `config.RealtimeMode`, `config.RealtimeConfig`, `config.ParseRuntimeRole(string)`, `config.ParseRealtimeMode(string)`, and `RuntimeRole.Runs(config.RuntimeCapability) bool`.
- Consumed by: application wiring and every worker task below.

- [x] **Step 1: Write failing role and configuration tests**

```go
func TestParseRuntimeRoleRejectsUnknown(t *testing.T) {
    _, err := ParseRuntimeRole("worker")
    require.ErrorContains(t, err, "invalid NOTIFUSE_ROLE")
}

func TestPrimaryRequiresRabbitMQ(t *testing.T) {
    cfg := RealtimeConfig{Mode: RealtimeModePrimary}
    require.ErrorContains(t, cfg.Validate(false), "RABBITMQ_URL")
}

func TestAPIRoleDoesNotRunBackgroundWorkers(t *testing.T) {
    assert.True(t, config.RoleAPI.Runs(config.CapabilityHTTP))
    assert.False(t, config.RoleAPI.Runs(config.CapabilityJourney))
}
```

- [x] **Step 2: Run the focused tests and verify RED**

Run:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./config ./internal/app -run 'Test(ParseRuntimeRole|PrimaryRequires|APIRole)'"
```

Expected: compilation fails because the realtime types and role policy do not exist.

- [x] **Step 3: Implement the minimal typed configuration**

```go
type RuntimeRole string
type RealtimeMode string
type RuntimeCapability string

type RealtimeConfig struct {
    Role RuntimeRole
    Mode RealtimeMode
    RabbitMQ RabbitMQConfig
    Redis RedisConfig
    ClickHouse ClickHouseConfig
    ObjectStore ObjectStoreConfig
    JourneyLease time.Duration
    JourneyHeartbeat time.Duration
    OutboxBatchSize int
    OutboxLease time.Duration
}

func (c RealtimeConfig) Validate(production bool) error
func (r RuntimeRole) Runs(capability RuntimeCapability) bool
```

Register every environment key from the spec in Viper, parse strict enums, and add `Realtime RealtimeConfig` to `Config`.

- [x] **Step 4: Run config/app tests and verify GREEN**

Run the Step 2 command, then:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./config ./internal/app"
```

- [x] **Step 5: Commit**

```powershell
git add config/realtime.go config/realtime_test.go config/config.go config/config_test.go internal/app/role.go internal/app/role_test.go
git commit -m "feat: add realtime runtime configuration"
```

### Task 2: Full Docker Compose infrastructure

**Files:**
- Create: `scripts/validate-realtime-compose.ps1`
- Modify: `compose.yaml`
- Create: `deploy/clickhouse/init/001_events.sql`
- Create: `deploy/rabbitmq/definitions.json`
- Create: `deploy/minio/init.sh`
- Modify: `env.example`
- Test: `scripts/validate-realtime-compose.ps1`

**Interfaces:**
- Consumes: configuration names from Task 1.
- Produces: healthy service names `postgres`, `pgbouncer`, `rabbitmq`, `redis`, `clickhouse`, `minio`, and process services for every runtime role.

- [x] **Step 1: Write the failing Compose structure test**

```powershell
$rendered = docker compose config --format json | ConvertFrom-Json
$required = 'postgres','pgbouncer','rabbitmq','redis','clickhouse','minio','api','outbox-relay','rule-worker','journey-worker','delivery-worker','analytics-worker','scheduler'
foreach ($name in $required) {
  if (-not $rendered.services.PSObject.Properties.Name.Contains($name)) { throw "missing service: $name" }
}
if ($rendered.services.api.environment.NOTIFUSE_ROLE -ne 'api') { throw 'api role mismatch' }
if ($rendered.services.pgbouncer.environment.POOL_MODE -ne 'transaction') { throw 'PgBouncer must use transaction pooling' }
```

- [x] **Step 2: Run the test and verify RED**

Run: `powershell -ExecutionPolicy Bypass -File scripts/validate-realtime-compose.ps1`

Expected: failure naming `pgbouncer` as the first missing service.

- [x] **Step 3: Add the infrastructure and role services**

Use these image families and responsibilities:

```yaml
pgbouncer: { image: edoburu/pgbouncer:v1.23.1-p3 }
rabbitmq: { image: rabbitmq:4-management }
redis: { image: redis:7-alpine }
clickhouse: { image: clickhouse/clickhouse-server:24.9 }
minio: { image: quay.io/minio/minio }
```

Route every Notifuse role through PgBouncer port 6432 using pgx named prepared statements, keep PostgreSQL port 5432 internal, configure health-conditioned dependencies, persistent volumes, one-shot RabbitMQ definition and MinIO bucket initialization, and make workers wait for the API migration gate. Do not expose published ports outside localhost in the default development Compose file.

- [x] **Step 4: Verify Compose rendering and container health**

Run:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/validate-realtime-compose.ps1
docker compose config --quiet
docker compose up -d postgres pgbouncer rabbitmq redis clickhouse minio
docker compose ps
```

Expected: validation exits 0 and all six dependencies become healthy.

- [x] **Step 5: Commit**

```powershell
git add compose.yaml env.example deploy scripts/validate-realtime-compose.ps1
git commit -m "feat: add realtime infrastructure topology"
```

### Task 3: Realtime domain contracts

**Files:**
- Create: `internal/domain/realtime.go`
- Create: `internal/domain/realtime_test.go`

**Interfaces:**
- Produces: `EventEnvelope`, `EventSubject`, `OutboxMessage`, `InboxClaim`, `TriggerBinding`, `MatchAudit`, `SideEffectExecution`, status enums, and `BuildSideEffectKey(workspaceID, contactAutomationID string, automationVersion int, nodeID string, executionVersion int64, channel string) string`.
- Consumed by: repository, broker, relay, matcher, journey, delivery, and analytics tasks.

- [x] **Step 1: Write failing serialization and idempotency tests**

```go
func TestEventEnvelopeRoundTripKeepsIdentity(t *testing.T) {
    original := EventEnvelope{ID: uuid.MustParse("018f0000-0000-7000-8000-000000000001"), Type: "contact.updated", SchemaVersion: 1}
    body, err := json.Marshal(original)
    require.NoError(t, err)
    var decoded EventEnvelope
    require.NoError(t, json.Unmarshal(body, &decoded))
    assert.Equal(t, original.ID, decoded.ID)
}

func TestBuildSideEffectKeyIsStable(t *testing.T) {
    first := BuildSideEffectKey("ws", "ca", 2, "node", 7, "email")
    second := BuildSideEffectKey("ws", "ca", 2, "node", 7, "email")
    assert.Equal(t, first, second)
    assert.Len(t, first, 64)
}
```

- [x] **Step 2: Run domain tests and verify RED**

Run:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/domain -run 'Test(EventEnvelope|BuildSideEffectKey)'"
```

- [x] **Step 3: Implement strict domain types and validation**

```go
type EventEnvelope struct {
    ID uuid.UUID `json:"id"`
    EventID uuid.UUID `json:"event_id"`
    Type string `json:"type"`
    SchemaVersion int `json:"schema_version"`
    WorkspaceID string `json:"workspace_id"`
    Subject EventSubject `json:"subject"`
    Source string `json:"source"`
    OccurredAt time.Time `json:"occurred_at"`
    ReceivedAt time.Time `json:"received_at"`
    CorrelationID uuid.UUID `json:"correlation_id"`
    CausationID *uuid.UUID `json:"causation_id,omitempty"`
    TraceID string `json:"trace_id,omitempty"`
    Data json.RawMessage `json:"data"`
}
```

Use canonical JSON hashing for payload conflict checks and SHA-256 for side-effect keys.

- [x] **Step 4: Run full domain tests and verify GREEN**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./internal/domain"`

- [x] **Step 5: Commit**

```powershell
git add internal/domain/realtime.go internal/domain/realtime_test.go
git commit -m "feat: define realtime event contracts"
```

### Task 4: Migration v40 and fixed timeline event bridge

**Files:**
- Create: `internal/migrations/v40.go`
- Create: `internal/migrations/v40_test.go`
- Modify: `internal/database/init.go`
- Modify: `internal/database/init_test.go`
- Modify: `internal/database/schema/automation_triggers.go`
- Modify: `internal/database/schema/automation_triggers_test.go`
- Modify: `config/config.go`
- Modify: `CHANGELOG.md`
- Test: `tests/integration/v40_realtime_migration_test.go`

**Interfaces:**
- Produces: workspace tables and functions from spec section 7; `contact_timeline.origin_event_id`; trigger function `notifuse_capture_timeline_event()`; migration version `40.0`.
- Consumed by: every runtime repository and worker.

- [x] **Step 1: Write failing migration SQL contract tests**

Assert exact schema invariants, not SQL formatting:

```go
func TestV40WorkspaceMigrationDefinesRealtimeTables(t *testing.T) {
    sql := V40Migration{}.workspaceSQL("workspace-123")
    for _, table := range []string{"event_idempotency", "event_ledger", "event_outbox", "consumer_inbox", "automation_trigger_bindings", "automation_match_audit", "side_effect_executions"} {
        assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS "+table)
    }
    assert.Contains(t, sql, "PARTITION BY RANGE (received_at)")
    assert.Contains(t, sql, "notifuse_capture_timeline_event")
}
```

Integration test upgrades a real v39 workspace twice, verifies tables/constraints, inserts one timeline row, and asserts exactly one idempotency, ledger, and outbox row with the same origin event ID.

- [x] **Step 2: Run migration tests and verify RED**

Run:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./internal/migrations ./internal/database ./internal/database/schema -run 'TestV40|TestFresh.*Realtime'"
```

- [x] **Step 3: Implement v40 schema and bridge**

Implement `MajorMigrationInterface`, return `HasWorkspaceUpdate=true`, `HasSystemUpdate=false`, and regenerate every live automation trigger using the same per-automation savepoint and validation pattern as v38. The bridge inserts idempotency, partitioned ledger, and outbox in the timeline transaction with conflict-safe semantics.

- [ ] **Step 4: Verify migration unit and integration tests**

Run:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./internal/migrations ./internal/database ./internal/database/schema"
docker compose -f tests/compose.test.yaml up -d
docker run --rm --network notifuse_test_network -e INTEGRATION_TESTS=true -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test -tags integration ./tests/integration -run TestV40RealtimeMigration -v"
```

- [x] **Step 5: Commit**

```powershell
git add internal/migrations/v40.go internal/migrations/v40_test.go internal/database/init.go internal/database/init_test.go internal/database/schema/automation_triggers.go internal/database/schema/automation_triggers_test.go config/config.go CHANGELOG.md tests/integration/v40_realtime_migration_test.go
git commit -m "feat: add realtime event schema migration"
```

### Task 5: Atomic PostgreSQL outbox, inbox, rule, and idempotency repositories

**Files:**
- Create: `internal/domain/mocks/mock_realtime_repository.go`
- Create: `internal/repository/realtime_postgres.go`
- Create: `internal/repository/realtime_postgres_test.go`
- Test: `tests/integration/realtime_repository_test.go`

**Interfaces:**
- Produces: `RealtimeRepository` methods `ClaimOutbox`, `MarkOutboxPublished`, `ReleaseOutbox`, `ClaimInbox`, `CompleteInbox`, `ListTriggerBindings`, `WriteMatchAudit`, `ReserveSideEffect`, and `GetEvent`.
- Consumed by: relay, matcher, journey, delivery, and projector.

- [x] **Step 1: Write failing SQL and concurrency tests**

```go
type RealtimeRepository interface {
    ClaimOutbox(ctx context.Context, workspaceID, workerID string, now time.Time, lease time.Duration, limit int) ([]domain.OutboxMessage, error)
    MarkOutboxPublished(ctx context.Context, workspaceID string, id, claimToken uuid.UUID, publishedAt time.Time) (bool, error)
    ClaimInbox(ctx context.Context, tx *sql.Tx, workspaceID, consumer string, messageID uuid.UUID, now time.Time, lease time.Duration) (domain.InboxClaim, error)
    CompleteInbox(ctx context.Context, tx *sql.Tx, workspaceID, consumer string, messageID, claimToken uuid.UUID, completedAt time.Time) (bool, error)
}
```

The integration race launches ten claimers against one outbox row and asserts one claim token wins.

- [x] **Step 2: Run repository tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -c "go test ./internal/repository -run TestRealtimeRepository"`

- [x] **Step 3: Implement atomic claims and generated mock**

Use one atomic statement for claims:

```sql
WITH candidates AS (
  SELECT id
  FROM event_outbox
  WHERE status = 'pending' AND available_at <= $1
  ORDER BY available_at, created_at
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE event_outbox AS o
SET status = 'claimed', claimed_by = $3, claim_token = $4,
    claim_expires_at = $1 + $5::interval, attempts = attempts + 1
FROM candidates
WHERE o.id = candidates.id
RETURNING o.*;
```

Every completion/release includes the claim token predicate. A completed inbox returns the duplicate-completed result without repeating business work.

- [ ] **Step 4: Run repository and concurrency tests**

Run repository unit tests, then `TestRealtimeRepositoryConcurrentClaim` under `-race` using the integration Compose database.

- [x] **Step 5: Commit**

```powershell
git add internal/domain/mocks/mock_realtime_repository.go internal/repository/realtime_postgres.go internal/repository/realtime_postgres_test.go tests/integration/realtime_repository_test.go
git commit -m "feat: add realtime persistence claims"
```

### Task 6: RabbitMQ topology and confirmed publisher

**Files:**
- Create: `pkg/broker/broker.go`
- Create: `pkg/broker/rabbitmq.go`
- Create: `pkg/broker/rabbitmq_test.go`
- Create: `pkg/broker/topology.go`
- Create: `pkg/broker/topology_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `Publisher.Publish(ctx, Message) error`, `Consumer.Consume(ctx, queue, Handler) error`, `Topology.Declare(context.Context) error`, delivery `Ack/Retry/DeadLetter` decisions.
- Consumed by: relay and all consumers.

- [x] **Step 1: Write failing topology and confirm tests**

```go
func TestTopologyUsesQuorumQueuesAndManualAck(t *testing.T) {
    declarations := DefaultTopology()
    require.NotEmpty(t, declarations.Queues)
    for _, q := range declarations.Queues { assert.Equal(t, "quorum", q.Arguments["x-queue-type"]) }
}

func TestPublisherReturnsErrorWhenConfirmTimesOut(t *testing.T) {
    publisher := NewPublisher(fakeConfirmChannel{confirm: nil}, 10*time.Millisecond)
    err := publisher.Publish(context.Background(), Message{ID: uuid.New(), RoutingKey: "contact.updated"})
    require.ErrorIs(t, err, ErrPublishConfirmTimeout)
}
```

- [x] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./pkg/broker"`

- [x] **Step 3: Add `amqp091-go` and implement reconnect-safe confirms**

Declare the four exchanges, quorum queues, four fixed TTL retry tiers, and dead queues. Publish persistent messages with `message_id`, `correlation_id`, content type, schema version, and tracing headers. Treat channel/connection closure and negative/expired confirms as failure.

- [ ] **Step 4: Run broker unit and live integration tests**

Run `go test ./pkg/broker` in the Go container, then a live test against the Compose RabbitMQ that publishes, receives, NACKs to retry, and dead-letters after the configured attempt limit.

- [x] **Step 5: Commit**

```powershell
git add pkg/broker go.mod go.sum
git commit -m "feat: add confirmed RabbitMQ transport"
```

### Task 7: Fair transactional outbox relay

**Files:**
- Create: `internal/service/realtime_outbox_relay.go`
- Create: `internal/service/realtime_outbox_relay_test.go`
- Modify: `internal/domain/mocks/mock_workspace_repository.go`
- Modify: `internal/domain/mocks/mock_realtime_repository.go`

**Interfaces:**
- Consumes: `RealtimeRepository`, `domain.WorkspaceRepository`, and `broker.Publisher`.
- Produces: `OutboxRelay.Run(context.Context) error` and fair persistent workspace cursor behavior.

- [x] **Step 1: Write failing confirm, release, and fairness tests**

Test three workspaces where the first is permanently busy; two consecutive batches must include later workspaces. Test confirm failure leaves/requeues the row with backoff and the original message ID.

- [x] **Step 2: Run service tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/service -run TestOutboxRelay"`

- [x] **Step 3: Implement relay lifecycle**

```go
type OutboxRelay struct {
    workspaces domain.WorkspaceRepository
    repository RealtimeRepository
    publisher broker.Publisher
    workerID string
    batchSize int
    lease time.Duration
}

func (r *OutboxRelay) Run(ctx context.Context) error
func (r *OutboxRelay) ProcessOnce(ctx context.Context) (int, error)
```

Inject `workspace_id` into the envelope immediately before publishing; never persist it back into a different workspace database.

- [ ] **Step 4: Run service tests and verify GREEN**

Run the Step 2 command with `-race`.

- [x] **Step 5: Commit**

```powershell
git add internal/service/realtime_outbox_relay.go internal/service/realtime_outbox_relay_test.go internal/domain/mocks
git commit -m "feat: relay realtime outbox fairly"
```

### Task 8: Indexed rule binding compiler and shadow matcher

**Files:**
- Create: `internal/service/realtime_rule_compiler.go`
- Create: `internal/service/realtime_rule_compiler_test.go`
- Create: `internal/service/realtime_rule_worker.go`
- Create: `internal/service/realtime_rule_worker_test.go`
- Modify: `internal/service/automation_service.go`
- Modify: `internal/service/automation_service_test.go`
- Modify: `internal/repository/realtime_postgres.go`
- Modify: `internal/repository/realtime_postgres_test.go`

**Interfaces:**
- Produces: `TriggerBindingCompiler.Compile(*domain.Automation)`, transactional binding replacement on automation activation/update, and `RuleWorker.Handle(context.Context, broker.Delivery) error`.
- Consumed by: primary enrollment and shadow audit.

- [x] **Step 1: Write failing compiler and mode tests**

```go
func TestCompilerIndexesUpdatedFields(t *testing.T) {
    a := automationFor("contact.updated", []string{"language", "country"})
    binding, err := compiler.Compile(a)
    require.NoError(t, err)
    assert.ElementsMatch(t, []string{"changes.country", "changes.language"}, binding.DependencyKeys)
}

func TestShadowMatcherWritesAuditWithoutEnrollment(t *testing.T) {
    repo := newRuleWorkerFakeRepository()
    worker := NewRuleWorker(repo, RealtimeModeShadow, time.Minute)
    require.NoError(t, worker.Handle(context.Background(), eventDelivery("event-1")))
    require.Len(t, repo.audits, 1)
    require.Empty(t, repo.enrollments)
    require.Empty(t, repo.outbox)
}

func TestPrimaryMatcherCreatesOneEnrollmentForDuplicateMessage(t *testing.T) {
    repo := newRuleWorkerFakeRepository()
    worker := NewRuleWorker(repo, RealtimeModePrimary, time.Minute)
    delivery := eventDelivery("event-1")
    require.NoError(t, worker.Handle(context.Background(), delivery))
    require.NoError(t, worker.Handle(context.Background(), delivery))
    require.Len(t, repo.enrollments, 1)
    require.Len(t, repo.outbox, 1)
}
```

- [x] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/service -run 'Test(Compiler|ShadowMatcher|PrimaryMatcher)'"`

- [x] **Step 3: Implement candidate indexing and SQL parity evaluation**

Compile event kind, list/segment ID, updated-field dependencies, condition hash, trigger JSON, and a parameterized condition query that reuses `QueryBuilder.BuildTriggerCondition`. Candidate lookup is keyed by event/subject and dependency overlap. The matcher evaluates candidates only, writes `realtime` audit in shadow, and creates contact automation plus journey-command outbox in primary in the same transaction.

- [ ] **Step 4: Run automation and rule-worker tests**

Run focused tests, then full `./internal/service` tests.

- [x] **Step 5: Commit**

```powershell
git add internal/service/realtime_rule_* internal/service/automation_service.go internal/service/automation_service_test.go internal/repository/realtime_postgres.go internal/repository/realtime_postgres_test.go
git commit -m "feat: add indexed realtime rule matching"
```

### Task 9: Atomic journey leases and side-effect idempotency

**Files:**
- Modify: `internal/domain/automation.go`
- Modify: `internal/domain/automation_test.go`
- Modify: `internal/domain/mocks/mock_automation_repository.go`
- Modify: `internal/repository/automation_postgres.go`
- Modify: `internal/repository/automation_postgres_test.go`
- Modify: `internal/service/automation_executor.go`
- Modify: `internal/service/automation_executor_test.go`
- Create: `internal/service/realtime_journey_worker.go`
- Create: `internal/service/realtime_journey_worker_test.go`
- Create: `tests/integration/realtime_journey_concurrency_test.go`

**Interfaces:**
- Produces: `ClaimContactAutomation`, `RenewContactAutomationClaim`, `CommitContactAutomationState`, `ReleaseContactAutomationClaim`, and `JourneyWorker.Handle`.
- Consumed by: scheduler, executor, and delivery command generation.

- [ ] **Step 1: Write failing stale-worker and crash-replay tests**

The key tests assert: one of ten workers claims a row; a stale claim token cannot update state; state version increments once; replaying an email node reserves the same effect key and emits one delivery command.

- [ ] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/repository ./internal/service -run 'Test(ContactAutomationClaim|JourneyWorker|SideEffect)'"`

- [ ] **Step 3: Implement lease-aware execution**

```go
type ContactAutomationClaim struct {
    ContactAutomation domain.ContactAutomation
    ClaimToken uuid.UUID
    StateVersion int64
    ExpiresAt time.Time
}
```

RabbitMQ only wakes the worker. PostgreSQL claim/version checks authorize every transition. Pure state changes and next-command outbox commit together. Delivery nodes reserve `side_effect_executions` before emitting a command.

- [ ] **Step 4: Run unit and integration concurrency tests**

Run focused unit tests with `-race`, then `TestRealtimeJourneyConcurrency` against real PostgreSQL.

- [ ] **Step 5: Commit**

```powershell
git add internal/domain/automation* internal/domain/mocks/mock_automation_repository.go internal/repository/automation_postgres* internal/service/automation_executor* internal/service/realtime_journey_worker* tests/integration/realtime_journey_concurrency_test.go
git commit -m "feat: make journey execution lease safe"
```

### Task 10: Redis cache and fail-closed frequency caps

**Files:**
- Create: `pkg/realtimecache/cache.go`
- Create: `pkg/realtimecache/redis.go`
- Create: `pkg/realtimecache/redis_test.go`
- Create: `internal/service/frequency_cap.go`
- Create: `internal/service/frequency_cap_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: versioned trigger cache and `FrequencyLimiter.Allow(ctx, workspaceID, subjectID, channel, policy, now) (Decision, error)`.
- Consumed by: rule and delivery workers.

- [ ] **Step 1: Write failing namespace, TTL, and outage tests**

Assert cache keys contain workspace and automation version, every write has TTL, and Redis failure returns `DecisionDefer` for frequency-controlled sends rather than `DecisionAllow`.

- [ ] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./pkg/realtimecache ./internal/service -run TestFrequency"`

- [ ] **Step 3: Implement with `go-redis/v9`**

Use a Lua script for atomic sliding-window caps and typed versioned cache payloads. Redis errors are observable and fall back to PostgreSQL for rules; frequency decisions fail closed as defer.

- [ ] **Step 4: Run unit and live Redis tests**

Run package/service tests and a Compose-backed expiration/concurrency test.

- [ ] **Step 5: Commit**

```powershell
git add pkg/realtimecache internal/service/frequency_cap* go.mod go.sum
git commit -m "feat: add distributed realtime frequency caps"
```

### Task 11: ClickHouse event projector

**Files:**
- Create: `pkg/eventanalytics/client.go`
- Create: `pkg/eventanalytics/clickhouse.go`
- Create: `pkg/eventanalytics/clickhouse_test.go`
- Create: `internal/service/realtime_analytics_worker.go`
- Create: `internal/service/realtime_analytics_worker_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `tests/integration/realtime_clickhouse_test.go`

**Interfaces:**
- Produces: `EventProjectionStore.InsertBatch(context.Context, []domain.EventEnvelope) error` and idempotent analytics consumer.
- Consumed by: analytics role only.

- [ ] **Step 1: Write failing batch, retry, and tenant-order tests**

Assert batches retain workspace ID, event type, subject, timestamps, and event ID; projector only completes inbox after ClickHouse returns success; duplicate event IDs produce one logical row in the reviewed query.

- [ ] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./pkg/eventanalytics ./internal/service -run TestAnalyticsWorker"`

- [ ] **Step 3: Implement `ReplacingMergeTree(projected_at)` writes**

Use async batching bounded by size and interval. No code path may write ClickHouse results back to PostgreSQL event ledger.

- [ ] **Step 4: Run unit and Compose ClickHouse integration tests**

Run focused tests plus `TestRealtimeClickHouseProjection`.

- [ ] **Step 5: Commit**

```powershell
git add pkg/eventanalytics internal/service/realtime_analytics_worker* tests/integration/realtime_clickhouse_test.go go.mod go.sum
git commit -m "feat: project realtime events to ClickHouse"
```

### Task 12: MinIO/S3 object storage

**Files:**
- Create: `pkg/objectstore/store.go`
- Create: `pkg/objectstore/s3.go`
- Create: `pkg/objectstore/s3_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `tests/integration/realtime_objectstore_test.go`

**Interfaces:**
- Produces: `ObjectStore.Put`, `Get`, `Delete`, `PresignGet`, and `WorkspaceObjectKey(workspaceID, assetID string, version int, filename string)`.
- Consumed by: future channel builders and existing file-manager integration adapter.

- [ ] **Step 1: Write failing key isolation and presign tests**

Assert path traversal is rejected, keys always begin with escaped workspace/asset/version components, and presigned URLs expire at the requested TTL without leaking credentials.

- [ ] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./pkg/objectstore"`

- [ ] **Step 3: Implement the small S3-compatible adapter**

Use `minio-go/v7`; do not put template metadata into object tags. Return typed unavailable/not-found/conflict errors.

- [ ] **Step 4: Run unit and MinIO integration tests**

Run package tests and `TestRealtimeObjectStore` against Compose MinIO.

- [ ] **Step 5: Commit**

```powershell
git add pkg/objectstore tests/integration/realtime_objectstore_test.go go.mod go.sum
git commit -m "feat: add S3-compatible asset storage"
```

### Task 13: Role-aware application lifecycle and workers

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `cmd/api/main.go`
- Modify: `cmd/api/main_test.go`
- Create: `internal/app/realtime_dependencies.go`
- Create: `internal/app/realtime_dependencies_test.go`

**Interfaces:**
- Consumes: all clients/services from Tasks 1–12.
- Produces: role-selective initialization, health/readiness state, graceful consumer shutdown, and no background workers in `api` role.

- [ ] **Step 1: Write failing lifecycle matrix tests**

For each role, assert exact initialized capabilities. Assert `api` can accept and persist events while RabbitMQ is down; `outbox-relay` readiness fails without RabbitMQ; `analytics-worker` health reports degraded ClickHouse without crashing unrelated roles; shutdown stops consumers before closing database clients.

- [ ] **Step 2: Run tests and verify RED**

Run: `docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/app ./cmd/api -run 'Test.*Role|TestRealtimeDependencies'"`

- [ ] **Step 3: Refactor initialization by capability**

Keep `NewApp` compatibility. Add role-aware dependency factories in a focused file, initialize only required repositories/services, and register HTTP handlers only for roles that run `CapabilityHTTP`. Start/stop workers through one lifecycle registry.

- [ ] **Step 4: Run app, command, service, and package tests**

Run:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test ./internal/app ./cmd/api ./internal/service ./pkg/..."
```

- [ ] **Step 5: Commit**

```powershell
git add internal/app/app.go internal/app/app_test.go internal/app/realtime_dependencies.go internal/app/realtime_dependencies_test.go cmd/api/main.go cmd/api/main_test.go
git commit -m "feat: run Notifuse as independent realtime roles"
```

### Task 14: Shadow comparison, primary cutover, and end-to-end verification

**Files:**
- Create: `internal/service/realtime_reconciliation.go`
- Create: `internal/service/realtime_reconciliation_test.go`
- Modify: `internal/service/automation_service.go`
- Modify: `internal/service/automation_service_test.go`
- Create: `tests/integration/realtime_e2e_test.go`
- Create: `tests/integration/realtime_failure_recovery_test.go`
- Create: `scripts/realtime-load-test.ps1`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: exact-event shadow mismatch summaries, `primary` activation/deactivation of per-automation triggers, rollback-safe mode transitions, and operational verification scripts.

- [ ] **Step 1: Write failing cutover and recovery tests**

Cover: legacy and realtime decisions share origin event ID; shadow never emits journey commands; primary disables dynamic triggers and enrolls once; reverting to legacy reinstalls validated triggers; duplicate delivery after crash is suppressed; RabbitMQ outage accumulates and later drains outbox.

- [ ] **Step 2: Run focused integration tests and verify RED**

Run:

```powershell
docker run --rm --network notifuse_default -e INTEGRATION_TESTS=true -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test -tags integration ./tests/integration -run 'TestRealtime(E2E|FailureRecovery|Cutover)' -v"
```

- [ ] **Step 3: Implement reconciliation and mode-controlled trigger lifecycle**

`legacy` installs dynamic triggers, `shadow` retains them and writes realtime audit only, and `primary` drops them after readiness validation. No mode transition deletes ledger, outbox, inbox, audit, or journey data.

- [ ] **Step 4: Run full verification**

Run:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/validate-realtime-compose.ps1
docker compose up -d --build
docker compose ps
docker run --rm --network notifuse_default -e INTEGRATION_TESTS=true -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test -race ./internal/... ./pkg/..."
docker run --rm --network notifuse_default -e INTEGRATION_TESTS=true -v "${PWD}:/src" -w /src golang:1.25-alpine sh -lc "go test -race -tags integration -timeout 20m ./tests/integration/..."
powershell -ExecutionPolicy Bypass -File scripts/realtime-load-test.ps1
```

Required evidence: all containers healthy; unit/integration suites pass; duplicate side effects remain zero; p95 ingest <200ms; p95 event-to-enrollment <2s; p99 <5s; no tenant starvation in the mixed load case.

- [ ] **Step 5: Commit**

```powershell
git add internal/service/realtime_reconciliation* internal/service/automation_service* tests/integration/realtime_e2e_test.go tests/integration/realtime_failure_recovery_test.go scripts/realtime-load-test.ps1 README.md CHANGELOG.md
git commit -m "feat: complete realtime runtime cutover"
```

## Plan Self-Review Record

- Spec coverage: Tasks 1–14 cover roles/config, all six dependencies, v40 tables/bridge, messaging, indexed matching, leases, idempotency, projection, object storage, mode switching, observability through health/metrics assertions, and failure/load verification.
- Scope control: new channels, new UI editors, identity graph APIs, Kafka, and MongoDB remain excluded.
- Type consistency: `RealtimeRepository`, `EventEnvelope`, `OutboxRelay`, `RuleWorker`, `JourneyWorker`, `EventProjectionStore`, and `ObjectStore` are introduced before their consumers.
- Execution choice: user explicitly requested direct implementation; use inline execution with `superpowers:executing-plans` and no subagents.
