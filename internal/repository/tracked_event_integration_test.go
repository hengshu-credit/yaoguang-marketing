//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func trackedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("requires isolated integration PostgreSQL")
	}
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		t.Fatal("TEST_DB_HOST required")
	}
	port := os.Getenv("TEST_DB_PORT")
	if port == "" {
		port = "5432"
	}
	dsn := fmt.Sprintf("host=%s port=%s user=notifuse_test password=test_password dbname=postgres sslmode=disable", host, port)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "events_" + uuid.New().String()[:8]
	if _, err = admin.Exec(`CREATE DATABASE ` + pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		admin.Exec(`DROP DATABASE ` + pq.QuoteIdentifier(name) + ` WITH (FORCE)`)
		admin.Close()
	})
	ddl := []string{`CREATE TABLE customers(id UUID PRIMARY KEY,customer_no TEXT UNIQUE NOT NULL,external_user_id TEXT UNIQUE,merged_into_id UUID,version BIGINT DEFAULT 1,created_at TIMESTAMPTZ DEFAULT NOW(),updated_at TIMESTAMPTZ DEFAULT NOW())`, `CREATE TABLE contacts(email TEXT PRIMARY KEY,customer_id UUID)`, `CREATE TABLE customer_identities(id UUID PRIMARY KEY,customer_id UUID REFERENCES customers(id),identity_type TEXT,value_ciphertext TEXT,lookup_fingerprint TEXT,display_hint TEXT,verified BOOLEAN,is_primary BOOLEAN,enabled BOOLEAN,metadata JSONB,created_at TIMESTAMPTZ,updated_at TIMESTAMPTZ,UNIQUE(identity_type,lookup_fingerprint))`, `CREATE TABLE customer_profiles(customer_id UUID PRIMARY KEY REFERENCES customers(id),attributes JSONB DEFAULT '{}',version BIGINT DEFAULT 1,updated_at TIMESTAMPTZ DEFAULT NOW())`}
	ddl = append(ddl, schema.BusinessConfigTableDefinitions()...)
	ddl = append(ddl, schema.TrackedEventTableDefinitions()...)
	ddl = append(ddl, schema.SubjectStateTableDefinitions()...)
	for _, statement := range schema.RealtimeTableDefinitions() {
		if strings.HasPrefix(statement, "ALTER TABLE") {
			break
		}
		ddl = append(ddl, statement)
	}
	ddl = append(ddl, `ALTER TABLE event_ledger ADD COLUMN customer_id UUID`, `CREATE TABLE event_ledger_default PARTITION OF event_ledger DEFAULT`)
	for _, statement := range ddl {
		if _, err = db.Exec(statement); err != nil {
			t.Fatalf("DDL: %v\n%s", err, statement)
		}
	}
	return db
}
func TestTrackedEventPersistenceConcurrentReceipt(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	defs := NewBusinessConfigRepositoryWithDB(db)
	object, err := defs.Save(ctx, "test", &domain.BusinessDefinition{Kind: "object_type", Key: "contract", Name: "Contract", Status: "active", Schema: map[string]any{"type": "object"}})
	if err != nil {
		t.Fatal(err)
	}
	if object.Key != "contract" {
		t.Fatal(object)
	}
	d := &domain.BusinessDefinition{Kind: "event", Key: "contract.signed", Name: "Signed", Revision: 1, Settings: map[string]any{"subject_type": "contract", "subject_id_path": "$.data.contract_id", "unmatched_customer": "create"}}
	now := time.Now().UTC()
	event := domain.TrackedEvent{ReportID: "same-report", EventCode: d.Key, EventName: d.Name, CustomerAttributes: map[string]any{"external_user_id": "same-customer"}, Data: map[string]any{"contract_id": "C-1"}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	var wg sync.WaitGroup
	results := make(chan *domain.TrackedEventResult, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := event
			result, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "test", WorkspaceSequence: 1, Source: "erp", PayloadHash: "original", Event: &copy, Definition: d})
			if err != nil {
				errs <- err
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Error(err)
	}
	var first string
	nonduplicates := 0
	for result := range results {
		if first == "" {
			first = result.EventID
		}
		if result.EventID != first {
			t.Error("different event IDs")
		}
		if !result.Duplicate {
			nonduplicates++
		}
		if !result.ReportedAt.Equal(now) {
			t.Error("reported_at changed")
		}
	}
	if nonduplicates != 1 {
		t.Errorf("first accepts=%d", nonduplicates)
	}
	for _, table := range []string{"event_report_receipts", "event_ledger", "event_outbox", "business_objects", "customers"} {
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM ` + pq.QuoteIdentifier(table)).Scan(&count); err != nil || count != 1 {
			t.Errorf("%s count=%d error=%v", table, count, err)
		}
	}
	copy := event
	_, err = repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "test", WorkspaceSequence: 1, Source: "erp", PayloadHash: "changed", Event: &copy, Definition: d})
	if err != domain.ErrEventPayloadConflict {
		t.Errorf("expected conflict got %v", err)
	}
	stored, hash, err := repo.GetReceipt(ctx, "test", "erp", event.ReportID)
	if err != nil || hash != "original" || stored.Event.Data["contract_id"] != "C-1" {
		t.Fatalf("original lost: %v %v", stored, err)
	}
	var envelope domain.EventEnvelope
	var raw []byte
	if err = db.QueryRow(`SELECT payload FROM event_outbox`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || envelope.Subject.CustomerID != stored.CustomerID {
		t.Fatalf("outbox mismatch %v", err)
	}
}
func TestTrackedEventPersistenceRollbackIdentityConflict(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	_, err := db.Exec(`INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,'one','one'),($2,'two','two')`, a, b)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	event := &domain.TrackedEvent{ReportID: "conflict", EventCode: "test", CustomerAttributes: map[string]any{"external_user_id": "one", "customer_id": b.String()}, Data: map[string]any{}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
	_, err = repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "test", WorkspaceSequence: 1, Source: "erp", PayloadHash: "hash", Event: event, Definition: &domain.BusinessDefinition{Settings: map[string]any{"unmatched_customer": "create"}}})
	if err == nil {
		t.Fatal("identity conflict accepted")
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM event_report_receipts`).Scan(&count)
	if count != 0 {
		t.Fatal("failed event left receipt")
	}
}
func TestTrackedEventPersistenceMultipleOccurrences(t *testing.T) {
	db := trackedTestDB(t)
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	ctx := context.Background()
	d := &domain.BusinessDefinition{Key: "login", Settings: map[string]any{"unmatched_customer": "create"}}
	now := time.Now().UTC()
	var customerID string
	for i := 0; i < 3; i++ {
		e := &domain.TrackedEvent{ReportID: fmt.Sprintf("login-occurrence-%d", i), EventCode: "login", CustomerAttributes: map[string]any{"external_user_id": "same-person"}, Data: map[string]any{}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
		command := domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "app", PayloadHash: e.ReportID, Event: e, Definition: d}
		result, err := repo.Ingest(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if customerID == "" {
			customerID = result.CustomerID
		}
		if result.CustomerID != customerID {
			t.Fatal("new customer for occurrence")
		}
		retry, err := repo.Ingest(ctx, command)
		if err != nil || !retry.Duplicate || retry.EventID != result.EventID {
			t.Fatalf("retry %+v %v", retry, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM event_ledger WHERE event_type='login' AND customer_id=$1`, customerID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("occurrences=%d error=%v", count, err)
	}
}
func TestBusinessConfigPersistenceIsolationRevisionAndObjectCardinality(t *testing.T) {
	db := trackedTestDB(t)
	other := trackedTestDB(t)
	ctx := context.Background()
	repo := NewBusinessConfigRepositoryWithDB(db)
	d := &domain.BusinessDefinition{Kind: "object_type", Key: "contract", Name: "Contract", Status: "active", Schema: map[string]any{"type": "object"}}
	saved, err := repo.Save(ctx, "w", d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, "w", d); err != domain.ErrBusinessDefinitionConflict {
		t.Fatalf("key conflict %v", err)
	}
	if _, err = NewBusinessConfigRepositoryWithDB(other).Save(ctx, "other", d); err != nil {
		t.Fatal("cross-workspace conflict", err)
	}
	stale := *saved
	saved.Name = "New"
	updated, err := repo.Save(ctx, "w", saved)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("revision %+v %v", updated, err)
	}
	if _, err = repo.Save(ctx, "w", &stale); err != domain.ErrBusinessDefinitionConflict {
		t.Fatalf("stale overwrite %v", err)
	}
	var original string
	if err = db.QueryRow(`SELECT document->>'name' FROM business_definition_revisions WHERE definition_id=$1 AND revision=1`, saved.ID).Scan(&original); err != nil || original != "Contract" {
		t.Fatalf("immutable revision %s %v", original, err)
	}
	customer := uuid.New()
	db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'one')`, customer)
	objects := NewBusinessObjectRepositoryWithDB(db)
	a, err := objects.Upsert(ctx, "w", &domain.BusinessObject{ObjectType: "contract", Source: "erp", ExternalKey: "same", CustomerID: customer.String(), Attributes: map[string]any{}, DefinitionRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := objects.Upsert(ctx, "w", &domain.BusinessObject{ObjectType: "invoice", Source: "erp", ExternalKey: "same", CustomerID: customer.String(), Attributes: map[string]any{}, DefinitionRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	c, err := objects.Upsert(ctx, "w", &domain.BusinessObject{ObjectType: "contract", Source: "erp", ExternalKey: "other", CustomerID: customer.String(), Attributes: map[string]any{}, DefinitionRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	relation := &domain.BusinessDefinition{Key: "contract_invoice", Settings: map[string]any{"from_type": "contract", "to_type": "invoice", "cardinality": "one_to_many"}}
	if err = objects.Relate(ctx, "w", relation, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err = objects.Relate(ctx, "w", relation, c.ID, b.ID); err == nil {
		t.Fatal("multiple parents accepted")
	}
}
func TestBrowserEventPersistenceSignedSessionAssociationKeepsOccurrence(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	e := &domain.TrackedEvent{ReportID: "anonymous-occurrence", EventCode: "register.view", CustomerAttributes: map[string]any{"browser_session_id": "session-one"}, Data: map[string]any{}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
	result, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "browser:https://shop.example", PayloadHash: "raw", Event: e, Definition: &domain.BusinessDefinition{Settings: map[string]any{"unmatched_customer": "pending"}}})
	if err != nil || result.ProcessingStatus != "pending_identity" {
		t.Fatalf("anonymous pending %+v %v", result, err)
	}
	id := uuid.New()
	db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'identified')`, id)
	n, err := repo.AssociateBrowserSession(ctx, "w", "browser:https://shop.example", "wrong-session", id.String(), []string{"register.view"})
	if err != nil || n != 0 {
		t.Fatalf("wrong session associated %d %v", n, err)
	}
	n, err = repo.AssociateBrowserSession(ctx, "w", "browser:https://shop.example", "session-one", id.String(), []string{"register.view"})
	if err != nil || n != 1 {
		t.Fatalf("association %d %v", n, err)
	}
	var customer, subject string
	if err = db.QueryRow(`SELECT customer_id,subject_id FROM event_ledger WHERE id=$1`, result.EventID).Scan(&customer, &subject); err != nil || customer != id.String() || subject != id.String() {
		t.Fatalf("ledger association %s %s %v", customer, subject, err)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM event_outbox`).Scan(&count)
	if count != 1 {
		t.Fatal("historical association repeated marketing outbox")
	}
	n, err = repo.AssociateBrowserSession(ctx, "w", "browser:https://shop.example", "session-one", id.String(), []string{"register.view"})
	if err != nil || n != 0 {
		t.Fatal("association is not idempotent")
	}
	if _, err = db.Exec(`DELETE FROM customers WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	e.ReportID = "new-occurrence-from-erased-session"
	_, err = repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "browser:https://shop.example", PayloadHash: "new-raw", Event: e, Definition: &domain.BusinessDefinition{Settings: map[string]any{"unmatched_customer": "pending"}}})
	if err != domain.ErrTrackedEventErased {
		t.Fatalf("erased browser session could be recreated: %v", err)
	}
}
func TestTrackedEventPersistenceResolvePendingFrozenExactlyOnce(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	defs := NewBusinessConfigRepositoryWithDB(db)
	_, err := defs.Save(ctx, "w", &domain.BusinessDefinition{Kind: "object_type", Key: "contract", Name: "Contract", Status: "active", Schema: map[string]any{"type": "object"}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	event := &domain.TrackedEvent{ReportID: "pending-one", EventCode: "contract.signed", EventName: "Original name", CustomerAttributes: map[string]any{"external_user_id": "pending-customer"}, Data: map[string]any{"contract_id": "C1", "score": float64(7)}, TriggeredAt: now.Add(-time.Minute), ReportedAt: now, ExpiresAt: &expiry, ProcessingStatus: "accepted", DefinitionRevision: 1, MappingRevision: 1}
	definition := &domain.BusinessDefinition{Kind: "event", Key: event.EventCode, Name: event.EventName, Revision: 1, Settings: map[string]any{"unmatched_customer": "pending", "subject_type": "contract", "subject_id_path": "$.data.contract_id"}}
	mapping := &domain.BusinessDefinition{Kind: "event_mapping", Key: "external", Revision: 1, Settings: map[string]any{"attribute_mappings": []any{map[string]any{"attribute": "score", "path": "$.data.score", "action": "update"}}}}
	original, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "erp", PayloadHash: "original", Event: event, Definition: definition, Mapping: mapping, Attributes: map[string]domain.BusinessDefinition{"score": {Key: "score", Revision: 1, Schema: map[string]any{"type": "number"}, Settings: map[string]any{"writable": true}}}})
	if err != nil || original.ProcessingStatus != "pending_identity" {
		t.Fatalf("pending %+v %v", original, err)
	}
	customer := uuid.New()
	if _, err = db.Exec(`INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,'resolved','pending-customer')`, customer); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE business_definitions SET status='archived',schema='{"type":"object","required":["new_required_field"]}' WHERE kind='object_type' AND key='contract'`)
	mapping.Settings["attribute_mappings"] = []any{}
	definition.Settings["subject_id_path"] = "$.data.wrong"
	var wg sync.WaitGroup
	results := make(chan *domain.TrackedEventResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.ResolvePending(ctx, "w", "erp", event.ReportID, customer.String(), "Customer identity verified", now.Add(time.Minute))
			if err != nil {
				errs <- err
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	first := 0
	for result := range results {
		if !result.Duplicate {
			first++
		}
		if result.EventID != original.EventID || !result.ReportedAt.Equal(original.ReportedAt) || result.Event.EventName != "Original name" {
			t.Fatal("original receipt interpretation changed")
		}
	}
	if first != 1 {
		t.Fatalf("release claims=%d", first)
	}
	var release, ledger, receipts int
	db.QueryRow(`SELECT count(*) FROM event_outbox WHERE routing_key='contract.signed.identity_resolved'`).Scan(&release)
	db.QueryRow(`SELECT count(*) FROM event_ledger`).Scan(&ledger)
	db.QueryRow(`SELECT count(*) FROM event_report_receipts`).Scan(&receipts)
	if release != 1 || ledger != 1 || receipts != 1 {
		t.Fatalf("release=%d ledger=%d receipts=%d", release, ledger, receipts)
	}
	var score string
	if err = db.QueryRow(`SELECT attributes->>'score' FROM customer_profiles WHERE customer_id=$1`, customer).Scan(&score); err != nil || score != "7" {
		t.Fatalf("original mapped update lost score=%s err=%v", score, err)
	}
}
func TestTrackedEventPersistenceResolveExpiredDoesNotRelease(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	expiry := now.Add(time.Second)
	e := &domain.TrackedEvent{ReportID: "expire-queued", EventCode: "login", CustomerAttributes: map[string]any{"external_user_id": "late"}, Data: map[string]any{}, TriggeredAt: now, ReportedAt: now, ExpiresAt: &expiry, ProcessingStatus: "accepted"}
	_, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "app", PayloadHash: "hash", Event: e, Definition: &domain.BusinessDefinition{Key: "login", Settings: map[string]any{"unmatched_customer": "pending"}}})
	if err != nil {
		t.Fatal(err)
	}
	customer := uuid.New()
	db.Exec(`INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,'late','late')`, customer)
	result, err := repo.ResolvePending(ctx, "w", "app", e.ReportID, "", "Identity now known", now.Add(time.Hour))
	if err != nil || result.ProcessingStatus != "expired" {
		t.Fatalf("expired %+v %v", result, err)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM event_outbox WHERE routing_key LIKE '%.identity_resolved'`).Scan(&count)
	if count != 0 {
		t.Fatal("expired event released")
	}
}
func TestTrackedEventPersistenceCustomerErasureRemovesHistoryAndBlocksRetry(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	e := &domain.TrackedEvent{ReportID: "erase-one", EventCode: "login", CustomerAttributes: map[string]any{"external_user_id": "erase-user"}, Data: map[string]any{"private": "payload"}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
	command := domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "app", PayloadHash: "hash", Event: e, Definition: &domain.BusinessDefinition{Key: "login", Settings: map[string]any{"unmatched_customer": "create"}}}
	result, err := repo.Ingest(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM customers WHERE id=$1`, result.CustomerID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"event_report_receipts", "event_ledger", "event_outbox", "subject_state_projections", "customer_external_id_aliases"} {
		var count int
		db.QueryRow(`SELECT count(*) FROM ` + pq.QuoteIdentifier(table)).Scan(&count)
		if count != 0 {
			t.Fatalf("%s retained erased data", table)
		}
	}
	if _, err = repo.Ingest(ctx, command); err != domain.ErrTrackedEventErased {
		t.Fatalf("erased report recreated customer: %v", err)
	}
}
func TestTrackedEventPersistenceMergedExternalAliasResolvesCanonicalUUID(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	source, target := uuid.New(), uuid.New()
	if _, err := db.Exec(`INSERT INTO customers(id,customer_no,external_user_id,merged_into_id) VALUES($1,'source','old-external',$2),($2,'target','new-external',NULL)`, source, target); err != nil {
		t.Fatal(err)
	}
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	for _, external := range []string{"old-external", "new-external"} {
		e := &domain.TrackedEvent{ReportID: external, EventCode: "login", CustomerAttributes: map[string]any{"external_user_id": external}, Data: map[string]any{}, TriggeredAt: now, ReportedAt: now, ProcessingStatus: "accepted"}
		result, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", WorkspaceSequence: 1, Source: "app", PayloadHash: external, Event: e, Definition: &domain.BusinessDefinition{Key: "login", Settings: map[string]any{"unmatched_customer": "reject"}}})
		if err != nil || result.CustomerID != target.String() {
			t.Fatalf("alias %s -> %+v %v", external, result, err)
		}
	}
}

func TestTrackedEventPersistenceHistoricalMappingsDoNotMutateIdentityOrProfile(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	id := uuid.New()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'historical-mapping')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_profiles(customer_id,attributes) VALUES($1,'{"score":1}')`, id); err != nil {
		t.Fatal(err)
	}
	mapping := &domain.BusinessDefinition{Key: "mapping", Settings: map[string]any{"attribute_mappings": []any{map[string]any{"attribute": "email", "path": "$.data.address", "action": "bind"}, map[string]any{"attribute": "score", "path": "$.data.score", "action": "update"}}}}
	attributes := map[string]domain.BusinessDefinition{"score": {Key: "score", Revision: 1, Schema: map[string]any{"type": "number"}, Settings: map[string]any{"writable": true}}}
	definition := &domain.BusinessDefinition{Key: "login", Settings: map[string]any{"unmatched_customer": "pending"}}
	for _, status := range []string{"expired", "backfill"} {
		expiry := now.Add(-time.Minute)
		e := &domain.TrackedEvent{ReportID: status, EventCode: "login", CustomerAttributes: map[string]any{"customer_id": id.String()}, Data: map[string]any{"address": status + "@example.com", "score": float64(9)}, ReportedAt: now, TriggeredAt: now.Add(-time.Hour), ExpiresAt: &expiry, ProcessingStatus: status, Backfill: status == "backfill"}
		if _, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", Source: "test", PayloadHash: status, Event: e, Definition: definition, Mapping: mapping, Attributes: attributes}); err != nil {
			t.Fatal(err)
		}
	}
	var identities int
	var score string
	if err := db.QueryRow(`SELECT COUNT(*) FROM customer_identities WHERE customer_id=$1`, id).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT attributes->>'score' FROM customer_profiles WHERE customer_id=$1`, id).Scan(&score); err != nil {
		t.Fatal(err)
	}
	if identities != 0 || score != "1" {
		t.Fatalf("historical mutated identities=%d score=%s", identities, score)
	}
	expires := now.Add(time.Minute)
	pending := &domain.TrackedEvent{ReportID: "pending-expired-map", EventCode: "login", CustomerAttributes: map[string]any{"external_user_id": "pending-external"}, Data: map[string]any{"address": "pending@example.com", "score": float64(22)}, ReportedAt: now, TriggeredAt: now, ExpiresAt: &expires, ProcessingStatus: "accepted"}
	first, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", Source: "test", PayloadHash: "pending", Event: pending, Definition: definition, Mapping: mapping, Attributes: attributes})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := repo.ResolvePending(ctx, "w", "test", pending.ReportID, id.String(), "Verified after expiry", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.EventID != first.EventID || resolved.ProcessingStatus != "expired" || !resolved.ReportedAt.Equal(first.ReportedAt) {
		t.Fatalf("resolution changed frozen occurrence %+v", resolved)
	}
	db.QueryRow(`SELECT COUNT(*) FROM customer_identities WHERE customer_id=$1`, id).Scan(&identities)
	db.QueryRow(`SELECT attributes->>'score' FROM customer_profiles WHERE customer_id=$1`, id).Scan(&score)
	var aliases int
	db.QueryRow(`SELECT COUNT(*) FROM customer_external_id_aliases WHERE external_user_id='pending-external'`).Scan(&aliases)
	if identities != 0 || aliases != 0 || score != "1" {
		t.Fatalf("expired resolution mutated identities=%d aliases=%d score=%s", identities, aliases, score)
	}
	live := &domain.TrackedEvent{ReportID: "live-map", EventCode: "login", CustomerAttributes: map[string]any{"customer_id": id.String()}, Data: map[string]any{"address": "live@example.com", "score": float64(9)}, ReportedAt: now, TriggeredAt: now, ProcessingStatus: "accepted"}
	command := domain.TrackedEventCommand{WorkspaceID: "w", Source: "test", PayloadHash: "live", Event: live, Definition: definition, Mapping: mapping, Attributes: attributes}
	if _, err = repo.Ingest(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Ingest(ctx, command); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM customer_identities WHERE customer_id=$1`, id).Scan(&identities)
	db.QueryRow(`SELECT attributes->>'score' FROM customer_profiles WHERE customer_id=$1`, id).Scan(&score)
	if identities != 1 || score != "9" {
		t.Fatalf("live mapping lost identities=%d score=%s", identities, score)
	}
}

func TestTrackedEventPersistenceHistoricalObjectAnchorAndExplicitLink(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	repo := NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef")
	defs := NewBusinessConfigRepositoryWithDB(db)
	now := time.Now().UTC()
	expiry := now.Add(-time.Minute)
	customer := uuid.New()
	if _, err := db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'historical-object')`, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := defs.Save(ctx, "w", &domain.BusinessDefinition{Kind: "object_type", Key: "contract", Name: "Contract", Status: "active", Schema: map[string]any{"type": "object"}}); err != nil {
		t.Fatal(err)
	}
	definition := &domain.BusinessDefinition{Key: "signed", Settings: map[string]any{"subject_type": "contract", "subject_id_path": "$.data.key", "subject_attributes_path": "$.data.attributes", "unmatched_customer": "pending"}}
	event := &domain.TrackedEvent{ReportID: "historical-object", EventCode: "signed", CustomerAttributes: map[string]any{"customer_id": customer.String()}, Data: map[string]any{"key": "old-contract", "attributes": map[string]any{"amount": float64(999)}}, ReportedAt: now, TriggeredAt: now.Add(-time.Hour), ExpiresAt: &expiry, ProcessingStatus: "expired"}
	first, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", Source: "test", PayloadHash: "historical-object", Event: event, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	var typ, objectID, attrs string
	if err = db.QueryRow(`SELECT subject_type,subject_id FROM event_ledger WHERE id=$1`, first.EventID).Scan(&typ, &objectID); err != nil {
		t.Fatal(err)
	}
	if typ != "contract" {
		t.Fatalf("historical object subject lost: %s", typ)
	}
	if err = db.QueryRow(`SELECT attributes::text FROM business_objects WHERE id=$1`, objectID).Scan(&attrs); err != nil || attrs != "{}" {
		t.Fatalf("historical current attrs=%s error=%v", attrs, err)
	}
	unknown := *event
	unknown.ReportID = "unlinked-history"
	unknown.CustomerAttributes = map[string]any{"external_user_id": "unknown-external"}
	unlinked, err := repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: "w", Source: "test", PayloadHash: "unlinked-history", Event: &unknown, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	if unlinked.CustomerID != "" {
		t.Fatal("created unknown historical customer")
	}
	linked, err := repo.ResolvePending(ctx, "w", "test", unknown.ReportID, customer.String(), "Verified historical association", now)
	if err != nil {
		t.Fatal(err)
	}
	if linked.CustomerID != customer.String() || linked.EventID != unlinked.EventID || !linked.ReportedAt.Equal(unlinked.ReportedAt) || linked.ProcessingStatus != "expired" {
		t.Fatalf("historical linking failed %+v", linked)
	}
	var releases, aliases int
	db.QueryRow(`SELECT count(*) FROM event_outbox WHERE routing_key LIKE '%.identity_resolved'`).Scan(&releases)
	db.QueryRow(`SELECT count(*) FROM customer_external_id_aliases WHERE external_user_id='unknown-external'`).Scan(&aliases)
	if releases != 0 || aliases != 0 {
		t.Fatalf("historical linking released=%d aliases=%d", releases, aliases)
	}
}
