package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/postgresdriver"
	"github.com/stretchr/testify/require"
)

func configurableRuntimeDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SNAPSHOT_POSTGRES_ADMIN_DSN")
	if dsn == "" {
		t.Skip("requires isolated SNAPSHOT_POSTGRES_ADMIN_DSN")
	}
	admin, err := sql.Open(postgresdriver.Name, dsn)
	require.NoError(t, err)
	name := "configurable_runtime_" + uuid.NewString()[:8]
	_, err = admin.Exec(`CREATE DATABASE "` + name + `"`)
	require.NoError(t, err)
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := sql.Open(postgresdriver.Name, parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close(); admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); admin.Close() })
	require.NoError(t, database.InitializeWorkspaceDatabase(db))
	return db
}

func TestConfiguredEventRuntimeRepeatedOccurrencesAndObjectFrequency(t *testing.T) {
	db := configurableRuntimeDB(t)
	ctx := context.Background()
	repo := repository.NewRealtimeRepositoryWithDB(db)
	worker, err := service.NewRuleWorker(repo, config.RealtimeModePrimary, time.Minute)
	require.NoError(t, err)
	worker.SetConfiguredOnly(true)
	customer := uuid.NewString()
	_, err = db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'no-email-customer')`, customer)
	require.NoError(t, err)
	compiler := service.NewTriggerBindingCompiler(service.NewQueryBuilder())
	for _, entry := range []struct {
		id, subject string
		once        bool
	}{{"every-event", "contract", false}, {"each-contract", "contract", true}, {"customer-view", "customer", false}} {
		trigger := &domain.TimelineTriggerConfig{EventKind: "tracked_event", Frequency: domain.TriggerFrequencyEveryTime, EventRule: &domain.MarketingEventRule{SubjectType: entry.subject, Events: []string{"contract.signed"}, OncePerObject: entry.once}}
		a := &domain.Automation{ID: entry.id, RootNodeID: "end", Trigger: trigger}
		bindings, err := compiler.CompileAll(a)
		require.NoError(t, err)
		raw, err := json.Marshal(trigger)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES($1,'ws',$1,'live',$2,'end','[{"id":"end","type":"delay","config":{"duration":1,"unit":"minutes"}}]',1)`, entry.id, raw)
		require.NoError(t, err)
		require.NoError(t, repo.ReplaceTriggerBindings(ctx, "ws", entry.id, bindings))
		loaded, err := repo.ListTriggerBindings(ctx, "ws", "contract.signed", entry.subject, nil)
		require.NoError(t, err)
		require.NotEmpty(t, loaded)
	}
	now := time.Now().UTC().Add(-time.Minute)
	emit := func(object string, expiry *time.Time, backfill bool) domain.EventEnvelope {
		status := "accepted"
		if backfill {
			status = "backfill"
		}
		raw, err := json.Marshal(map[string]any{"report_id": uuid.NewString(), "processing_status": status, "expires_at": expiry, "data": map[string]any{"contract_id": object}, "customer_attributes": map[string]any{"customer_uuid": customer}})
		require.NoError(t, err)
		e := domain.EventEnvelope{ID: uuid.New(), EventID: uuid.New(), WorkspaceID: "ws", Type: "contract.signed", SchemaVersion: 1, Subject: domain.EventSubject{Type: "contract", ID: object, CustomerID: customer}, Source: "erp", OccurredAt: now, ReceivedAt: now, CorrelationID: uuid.New(), Data: raw}
		_, err = repo.AppendEvent(ctx, "ws", e, now)
		require.NoError(t, err)
		return e
	}
	first := emit("contract-A", nil, false)
	emit("contract-A", nil, false)
	emit("contract-B", nil, false)
	expired := now.Add(-time.Second)
	emit("contract-C", &expired, false)
	emit("contract-D", nil, true)
	jobs, err := repo.ClaimConfiguredEvents(ctx, "ws", time.Now().Add(time.Second), time.Minute, 100)
	require.NoError(t, err)
	require.Len(t, jobs, 5)
	for _, job := range jobs {
		require.NoError(t, worker.Handle(ctx, broker.Message{ID: job.ID, Body: job.Payload}))
		require.NoError(t, repo.FinishConfiguredEvent(ctx, "ws", job, time.Now(), ""))
	}
	body, err := json.Marshal(first)
	require.NoError(t, err)
	require.NoError(t, worker.Handle(ctx, broker.Message{ID: first.ID, Body: body}))
	for id, want := range map[string]int{"every-event": 3, "each-contract": 2, "customer-view": 3} {
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM contact_automations WHERE automation_id=$1`, id).Scan(&count))
		require.Equal(t, want, count, id)
	}
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM event_ledger WHERE event_type='contract.signed'`).Scan(&count))
	require.Equal(t, 5, count)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM journey_event_subjects`).Scan(&count))
	require.Equal(t, 8, count)
	jobs, err = repo.ClaimConfiguredEvents(ctx, "ws", time.Now().Add(time.Hour), time.Minute, 100)
	require.NoError(t, err)
	require.Empty(t, jobs)
	var instance string
	require.NoError(t, db.QueryRow(`SELECT contact_automation_id FROM journey_event_subjects WHERE subject_id='contract-A' ORDER BY contact_automation_id LIMIT 1`).Scan(&instance))
	started := time.Now().UTC()
	now = started.Add(time.Second)
	emit("contract-B", nil, false)
	condition := domain.BusinessConditionNodeConfig{Mode: "wait", SubjectType: "contract", Events: []string{"contract.signed"}, TimeoutSeconds: 60, OnTimeout: "exit"}
	matched, _, err := repo.EvaluateJourneyBusinessCondition(ctx, "ws", instance, condition, started, now.Add(time.Second))
	require.NoError(t, err)
	require.False(t, matched, "other contract must not satisfy wait")
	emit("contract-A", nil, false)
	matched, evidence, err := repo.EvaluateJourneyBusinessCondition(ctx, "ws", instance, condition, started, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, "contract-A", evidence["subject_id"])
}

func TestConfiguredRuntimeChildConditionsUseActualObjectEvidence(t *testing.T) {
	db := configurableRuntimeDB(t)
	ctx := context.Background()
	repo := repository.NewRealtimeRepositoryWithDB(db)
	customer, a, b := uuid.NewString(), uuid.NewString(), uuid.NewString()
	at := time.Now().UTC().Add(-time.Hour)
	_, err := db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'child-customer')`, customer)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO business_objects(id,object_type,source,external_key,customer_id,attributes,definition_revision,created_at) VALUES($1,'contract','test','A',$3,'{"tier":"gold"}',1,$4),($2,'contract','test','B',$3,'{"tier":"silver"}',1,$4)`, a, b, customer, at)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE subject_attribute_history SET valid_from=$1,recorded_at=$1 WHERE customer_id=$2`, at, customer)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES('child-flow','ws','Child flow','live','{}','wait','[]',1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contact_automations(id,automation_id,customer_id,current_node_id,status,entered_at) VALUES('child-journey','child-flow',$1,'wait','active',$2)`, customer, at)
	require.NoError(t, err)
	emit := func(code, object string, occurred time.Time) {
		raw, _ := json.Marshal(map[string]any{"processing_status": "accepted", "data": map[string]any{}})
		_, err := repo.AppendEvent(ctx, "ws", domain.EventEnvelope{ID: uuid.New(), EventID: uuid.New(), WorkspaceID: "ws", Type: code, SchemaVersion: 1, Subject: domain.EventSubject{Type: "contract", ID: object, CustomerID: customer}, Source: "test", OccurredAt: occurred, ReceivedAt: occurred, CorrelationID: uuid.New(), Data: raw}, occurred)
		require.NoError(t, err)
	}
	emit("contract.signed", a, at.Add(time.Minute))
	emit("contract.approved", b, at.Add(2*time.Minute))
	condition := domain.BusinessConditionNodeConfig{Mode: "continue_if", SubjectType: "customer", Condition: &domain.StateRule{Kind: "object_any", SubjectType: "contract", Children: []domain.StateRule{{Kind: "event", Event: "contract.signed"}}}}
	matched, _, err := repo.EvaluateJourneyBusinessCondition(ctx, "ws", "child-journey", condition, at, at.Add(3*time.Minute))
	require.NoError(t, err)
	require.True(t, matched, "objects without state history still have event evidence")
	condition.Condition.Children = []domain.StateRule{{Kind: "all", Children: []domain.StateRule{{Kind: "event", Event: "contract.signed"}, {Kind: "event", Event: "contract.approved"}, {Kind: "attribute", Field: "tier", Operator: "eq", Operand: "gold"}}}}
	matched, _, err = repo.EvaluateJourneyBusinessCondition(ctx, "ws", "child-journey", condition, at, at.Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, matched, "two contracts must not satisfy one child conjunction")
	emit("contract.approved", a, at.Add(4*time.Minute))
	_, err = db.Exec(`UPDATE business_objects SET attributes='{"tier":"silver"}' WHERE id=$1`, a)
	require.NoError(t, err)
	matched, _, err = repo.EvaluateJourneyBusinessCondition(ctx, "ws", "child-journey", condition, at, at.Add(5*time.Minute))
	require.NoError(t, err)
	require.True(t, matched, "historical gold attribute and both occurrences belong to contract A")
}

func TestConfiguredRuntimeInitialStartChecksExpiryWithoutChangingFacts(t *testing.T) {
	db := configurableRuntimeDB(t)
	repo := repository.NewRealtimeRepositoryWithDB(db)
	ctx := context.Background()
	customer := uuid.NewString()
	eventID := uuid.New()
	reported := time.Now().UTC().Truncate(time.Microsecond)
	expiry := reported.Add(time.Minute)
	_, err := db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'expiry-customer')`, customer)
	require.NoError(t, err)
	raw, _ := json.Marshal(map[string]any{"processing_status": "accepted", "expires_at": expiry, "reported_at": reported, "data": map[string]any{}})
	_, err = repo.AppendEvent(ctx, "ws", domain.EventEnvelope{ID: uuid.New(), EventID: eventID, WorkspaceID: "ws", Type: "login", SchemaVersion: 1, Subject: domain.EventSubject{Type: "customer", ID: customer, CustomerID: customer}, Source: "test", OccurredAt: reported, ReceivedAt: reported, CorrelationID: uuid.New(), Data: raw}, reported)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO journey_event_subjects(contact_automation_id,subject_type,subject_id,customer_id,origin_event_id) VALUES('delayed-start','customer',$1::text,$1::uuid,$2)`, customer, eventID)
	require.NoError(t, err)
	reason, _, err := repo.ConfiguredJourneyStart(ctx, "ws", "delayed-start", reported)
	require.NoError(t, err)
	require.Empty(t, reason)
	reason, _, err = repo.ConfiguredJourneyStart(ctx, "ws", "delayed-start", expiry)
	require.NoError(t, err)
	require.Equal(t, "origin_event_expired", reason)
	var received time.Time
	var status string
	require.NoError(t, db.QueryRow(`SELECT received_at,properties->>'processing_status' FROM event_ledger WHERE id=$1`, eventID).Scan(&received, &status))
	require.True(t, reported.Equal(received))
	require.Equal(t, "accepted", status)
}
