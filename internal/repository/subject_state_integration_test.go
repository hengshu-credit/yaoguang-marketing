//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	_ "github.com/lib/pq"
)

func TestSubjectStateFunnelPostgresIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("requires isolated integration PostgreSQL")
	}
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "notifuse-configurable-postgres"
	}
	base := fmt.Sprintf("host=%s port=5432 user=notifuse_test password=test_password sslmode=disable", host)
	admin, err := sql.Open("postgres", base+" dbname=postgres")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "analysis_" + fmt.Sprint(time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", base+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	for _, ddl := range []string{`CREATE TABLE customers(id UUID PRIMARY KEY,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),merged_into_id UUID)`, `CREATE TABLE customer_profiles(customer_id UUID PRIMARY KEY REFERENCES customers(id),attributes JSONB NOT NULL DEFAULT '{}')`, `CREATE TABLE business_objects(id UUID PRIMARY KEY,object_type TEXT,customer_id UUID REFERENCES customers(id),attributes JSONB NOT NULL DEFAULT '{}',created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`, `CREATE TABLE event_ledger(id UUID,event_type TEXT,subject_type TEXT,subject_id TEXT,customer_id UUID,occurred_at TIMESTAMPTZ,received_at TIMESTAMPTZ,sequence BIGINT,properties JSONB)`} {
		if _, err = db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	for _, ddl := range schema.SubjectStateTableDefinitions() {
		if _, err = db.Exec(ddl); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	if _, err = db.Exec(`CREATE TABLE business_object_relations(id UUID PRIMARY KEY,relation_key TEXT,from_id UUID,to_id UUID,valid_from TIMESTAMPTZ,valid_to TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE business_definition_revisions(definition_id UUID,revision INTEGER,document JSONB,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range append(schema.SubjectAttributeHistoryTableDefinitions(), schema.FunnelTableDefinitions()...) {
		if _, err = db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	customer := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO customers(id) VALUES($1)`, customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO customer_profiles VALUES($1,'{"active":true}')`, customer); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	obj := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO business_objects(id,object_type,customer_id,attributes) VALUES($1,'contract',$2,'{}')`, obj, customer); err != nil {
		t.Fatal(err)
	}
	for i, code := range []string{"start", "start", "finish"} {
		at := now.Add(time.Duration(i) * time.Minute)
		if _, err = db.Exec(`INSERT INTO event_ledger VALUES($1,$2,'contract',$3,$4,$5,$5,NULL,$6)`, uuid.NewString(), code, obj, customer, at, `{"data":{"amount_minor":100,"currency":"CNY","transaction_id":"payment-1"}}`); err != nil {
			t.Fatal(err)
		}
	}
	r := NewSubjectStateRepositoryWithDB(db)
	expiry := now.Add(time.Hour)
	if err = r.SetOverride(ctx, name, "contract", obj, domain.StateOverride{StateKey: "eligible", Value: domain.StateTrue, Reason: "review", Actor: "user", ValidFrom: now, ValidTo: &expiry}); err != nil {
		t.Fatal(err)
	}
	overrides, err := r.Overrides(ctx, name, "contract", obj)
	if err != nil || len(overrides) != 1 {
		t.Fatalf("overrides %v %v", overrides, err)
	}
	h := domain.SubjectStateInterval{SubjectType: "contract", SubjectID: obj, CustomerID: customer, StateKey: "eligible", Value: domain.StateTrue, ValidFrom: now, ValidTo: &expiry, RuleRevision: 1, Source: "rule", ComputedAt: now}
	if err = r.ReplaceProjection(ctx, name, "contract", obj, []domain.SubjectStateInterval{h}, &expiry); err != nil {
		t.Fatal(err)
	}
	if err = r.ReplaceProjection(ctx, name, "contract", obj, []domain.SubjectStateInterval{h}, &expiry); err != nil {
		t.Fatal(err)
	}
	states, err := r.List(ctx, name, "contract", obj, true)
	if err != nil || len(states) != 1 || states[0].ProjectionRevision != 2 {
		t.Fatalf("projection read %+v %v", states, err)
	}
	var retained int
	if err = db.QueryRow(`SELECT count(*) FROM subject_state_history`).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("old projection erased: %d %v", retained, err)
	}
	due, err := r.Due(ctx, name, time.Now().Add(time.Hour), 100)
	if err != nil || len(due) != 2 {
		t.Fatalf("expiry scheduling %v %v", due, err)
	}
	if _, err = r.Context(ctx, name, "contract", obj); err != nil {
		t.Fatal(err)
	}
	attributeTx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = attributeTx.Exec(`SELECT set_config('notifuse.business_event_at',$1,true)`, now.Add(10*time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = attributeTx.Exec(`UPDATE customer_profiles SET attributes=attributes || '{"tier":"gold"}' WHERE customer_id=$1`, customer); err != nil {
		t.Fatal(err)
	}
	if err = attributeTx.Commit(); err != nil {
		t.Fatal(err)
	}
	attrs, err := r.AttributeHistory(ctx, name, "customer", customer, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	historicalAttrs := domain.SubjectAttributesAt(attrs, now.Add(11*time.Minute))
	if historicalAttrs["tier"] != "gold" {
		t.Fatalf("event-time attr missing: %+v", attrs)
	}
	if _, exists := historicalAttrs["active"]; exists {
		t.Fatal("unchanged current attribute leaked into past snapshot")
	}
	job := &domain.FunnelQueryJob{ID: uuid.NewString(), Status: "running", Query: domain.FunnelQuery{WorkspaceID: name, Start: now, End: expiry, Watermark: expiry}}
	jobRepo := NewFunnelRepositoryWithDB(db)
	if err = jobRepo.CreateQuery(ctx, name, job); err != nil {
		t.Fatal(err)
	}
	if err = jobRepo.FinishQuery(ctx, name, job.ID, "cancelled", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err = jobRepo.FinishQuery(ctx, name, job.ID, "completed", &domain.FunnelResult{Entered: 100}, ""); err != nil {
		t.Fatal(err)
	}
	storedJob, err := jobRepo.GetQuery(ctx, name, job.ID)
	if err != nil || storedJob.Status != "cancelled" || storedJob.Result != nil {
		t.Fatalf("cancel overwritten: %+v %v", storedJob, err)
	}
	f := NewFunnelRepositoryWithDB(db)
	events, err := f.Events(ctx, name, "contract", now, now.Add(time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	history, err := f.StateHistory(ctx, name, now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	definition := domain.FunnelDefinition{SubjectType: "contract", WindowSeconds: 3600, Steps: []domain.FunnelStep{{ID: "a", Events: []string{"start"}}, {ID: "b", Events: []string{"finish"}, StateFilters: map[string]domain.StateValue{"eligible": domain.StateTrue}}}}
	result, err := domain.ComputeFunnel(definition, domain.FunnelQuery{Start: now, End: now.Add(time.Minute * 2), Watermark: now.Add(time.Hour)}, events, history)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entered != 1 || result.Converted != 1 || result.Steps[0].Occurrences != 2 || result.Steps[1].AmountMinor["CNY"] != 100 {
		t.Fatalf("repeat events / state history / money incorrect: %+v", result)
	}
	revised := h
	revised.Value = domain.StateFalse
	revised.ComputedAt = time.Now().UTC()
	if err = r.ReplaceProjection(ctx, name, "contract", obj, []domain.SubjectStateInterval{revised}, nil); err != nil {
		t.Fatal(err)
	}
	watermarked, err := f.StateHistoryAt(ctx, name, now, expiry, expiry)
	if err != nil || len(watermarked) != 1 || watermarked[0].Value != domain.StateTrue {
		t.Fatalf("late projection leaked into saved watermark: %+v %v", watermarked, err)
	}
	if _, err = db.Exec(`CREATE TABLE execution_evidence(execution_id TEXT,customer_id UUID,kind TEXT,status TEXT,payload JSONB,occurred_at TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO execution_evidence VALUES('run-1',$1,'delivery_status','confirmed','{"message_id":"message-1"}',$2),('unsent',$1,'delivery_content','pending','{"message_id":"message-2"}',$2)`, customer, now); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO event_ledger VALUES($1,'email.clicked','customer',$2::text,$2::uuid,$3,$3,NULL,'{"message_id":"message-1"}')`, uuid.NewString(), customer, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	links, err := f.AttributionLinks(ctx, name, now, now.Add(time.Hour), now.Add(time.Hour))
	if err != nil || len(links) != 2 {
		t.Fatalf("attribution SQL %+v %v", links, err)
	}
	for _, link := range links {
		if link.RunID != "run-1" {
			t.Fatal("pending queue must not count as touchpoint")
		}
	}
	filtered := domain.FilterAnalysisScope(events, links, "attributed", "run-1", 3600)
	if len(filtered) != 2 {
		t.Fatalf("only events after verified click attributed: %d", len(filtered))
	}
	// A dirty marker arriving during a projection waits for the source-read lock;
	// after publication it must remain scheduled, not be cleared by stale work.
	arrivalStarted := make(chan struct{})
	arrivalDone := make(chan error, 1)
	err = r.WithSubjectLock(ctx, name, "contract", obj, func(locked context.Context) error {
		go func() {
			close(arrivalStarted)
			_, err := db.Exec(`UPDATE subject_state_projections SET next_check=NOW() WHERE subject_type='contract' AND subject_id=$1`, obj)
			arrivalDone <- err
		}()
		<-arrivalStarted
		select {
		case err := <-arrivalDone:
			return fmt.Errorf("dirty update escaped projection row lock: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
		return r.ReplaceProjection(locked, name, "contract", obj, []domain.SubjectStateInterval{h}, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-arrivalDone; err != nil {
		t.Fatal(err)
	}
	var remainsDue bool
	if err = db.QueryRow(`SELECT next_check IS NOT NULL FROM subject_state_projections WHERE subject_type='contract' AND subject_id=$1`, obj).Scan(&remainsDue); err != nil || !remainsDue {
		t.Fatalf("concurrent dirty update was lost: %v", err)
	}
	if _, err = db.Exec(`DELETE FROM customers WHERE id=$1`, customer); err == nil {
		t.Fatal("test object FK should prevent unsafe customer delete before object cleanup")
	}
	for _, ddl := range []string{`CREATE TABLE contacts(email TEXT,customer_id UUID)`, `CREATE TABLE event_idempotency(id UUID PRIMARY KEY,received_at TIMESTAMPTZ,payload_hash TEXT)`, `CREATE TABLE event_outbox(id UUID PRIMARY KEY,event_id UUID,topic TEXT,routing_key TEXT,payload JSONB,headers JSONB,created_at TIMESTAMPTZ DEFAULT NOW())`, `ALTER TABLE event_ledger ADD COLUMN contact_email TEXT,ADD COLUMN source TEXT,ADD COLUMN schema_version INTEGER,ADD COLUMN context JSONB`} {
		if _, err = db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	oldState := h
	oldState.ValidFrom = time.Now().Add(-time.Minute)
	oldState.ValidTo = nil
	if err = r.ReplaceProjection(ctx, name, "contract", obj, []domain.SubjectStateInterval{oldState}, nil); err != nil {
		t.Fatal(err)
	}
	newState := oldState
	newState.Value = domain.StateFalse
	newState.ValidFrom = time.Now().UTC()
	newState.Source = "manual"
	live := domain.WithStateTransitionEmission(ctx, domain.StateTransitionEmission{Enabled: true})
	if err = r.ReplaceProjection(live, name, "contract", obj, []domain.SubjectStateInterval{newState}, nil); err != nil {
		t.Fatal(err)
	}
	if err = r.ReplaceProjection(live, name, "contract", obj, []domain.SubjectStateInterval{newState}, nil); err != nil {
		t.Fatal(err)
	}
	var stateEvents, outboxCount int
	if err = db.QueryRow(`SELECT count(*) FROM event_ledger WHERE event_type='state.exited'`).Scan(&stateEvents); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM event_outbox`).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if stateEvents != 1 || outboxCount != 1 {
		t.Fatalf("state transition retry duplicated: ledger=%d outbox=%d", stateEvents, outboxCount)
	}
	newState.Value = domain.StateTrue
	newState.ValidFrom = time.Now()
	if err = r.ReplaceProjection(ctx, name, "contract", obj, []domain.SubjectStateInterval{newState}, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM event_outbox`).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("replay emitted state event: %d %v", outboxCount, err)
	}
	childID := uuid.NewString()
	beforeRelation := time.Now().UTC()
	if _, err = db.Exec(`INSERT INTO business_objects(id,object_type,customer_id,attributes) VALUES($1,'invoice',$2,'{}')`, childID, customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO business_object_relations VALUES($1,'contract_invoices',$2,$3,$4,NULL)`, uuid.NewString(), obj, childID, now); err != nil {
		t.Fatal(err)
	}
	childState := domain.SubjectStateInterval{SubjectType: "invoice", SubjectID: childID, CustomerID: customer, StateKey: "paid", Value: domain.StateTrue, ValidFrom: now, ComputedAt: now, RuleRevision: 1, Source: "rule"}
	if err = r.ReplaceProjection(ctx, name, "invoice", childID, []domain.SubjectStateInterval{childState}, nil); err != nil {
		t.Fatal(err)
	}
	relations, err := NewFunnelRepositoryWithDB(db).ObjectRelations(ctx, name, now, now.Add(time.Hour), time.Now())
	if err != nil || len(relations) != 1 || relations[0].ToID != childID {
		t.Fatal(relations, err)
	}
	pastRelations, err := NewFunnelRepositoryWithDB(db).ObjectRelations(ctx, name, now, now.Add(time.Hour), beforeRelation)
	if err != nil || len(pastRelations) != 0 {
		t.Fatalf("later-established relation leaked into historical watermark: %+v %v", pastRelations, err)
	}
	beforeUnlink := time.Now().UTC()
	if _, err = db.Exec(`UPDATE business_object_relations SET valid_to=$1 WHERE from_id=$2`, now.Add(30*time.Minute), obj); err != nil {
		t.Fatal(err)
	}
	oldRelations, err := NewFunnelRepositoryWithDB(db).ObjectRelations(ctx, name, now, now.Add(time.Hour), beforeUnlink)
	if err != nil || len(oldRelations) != 1 || oldRelations[0].ValidTo != nil {
		t.Fatalf("later unlink reinterpreted old watermark: %+v %v", oldRelations, err)
	}
	if _, err = db.Exec(`UPDATE business_object_relations SET valid_to=NULL WHERE from_id=$1`, obj); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO business_definition_revisions VALUES($1,1,'{"kind":"object_relation","key":"customer_contracts","status":"active","settings":{"from_type":"customer","to_type":"contract"}}',clock_timestamp())`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	ownership, err := NewFunnelRepositoryWithDB(db).ObjectRelations(ctx, name, now, time.Now().Add(time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ownerFound := false
	for _, link := range ownership {
		if link.Key == "customer_contracts" && link.FromID == customer && link.ToID == obj {
			ownerFound = true
		}
	}
	if !ownerFound {
		t.Fatal(ownership)
	}
	parentContext, err := r.Context(ctx, name, "contract", obj)
	if err != nil {
		t.Fatal(err)
	}
	rule := domain.StateRule{Kind: "object_any", SubjectType: "invoice", RelationKey: "contract_invoices", Children: []domain.StateRule{{Kind: "state", StateKey: "paid", Value: domain.StateTrue}}}
	if domain.EvaluateStateRule(rule, parentContext).Value != domain.StateTrue {
		t.Fatal(parentContext)
	}
	customerContext, err := r.Context(ctx, name, "customer", customer)
	if err != nil {
		t.Fatal(err)
	}
	ownerRule := domain.StateRule{Kind: "object_any", SubjectType: "contract", RelationKey: "customer_contracts", Children: []domain.StateRule{{Kind: "state", StateKey: "eligible", Value: domain.StateTrue}}}
	if domain.EvaluateStateRule(ownerRule, customerContext).Value != domain.StateTrue {
		t.Fatal(customerContext)
	}
	coverageAt := time.Now().UTC()
	coverageIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for _, id := range coverageIDs {
		if _, err = db.Exec(`INSERT INTO business_objects(id,object_type,customer_id,attributes,created_at) VALUES($1,'coverage_object',$2,'{}',$3)`, id, customer, now); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range coverageIDs[:2] {
		states := []domain.SubjectStateInterval{{SubjectType: "coverage_object", SubjectID: id, CustomerID: customer, StateKey: "X", Value: domain.StateTrue, ValidFrom: now, ComputedAt: coverageAt, RuleRevision: 1, Source: "rule"}, {SubjectType: "coverage_object", SubjectID: id, CustomerID: customer, StateKey: "Y", Value: domain.StateFalse, ValidFrom: now, ComputedAt: coverageAt, RuleRevision: 1, Source: "rule"}}
		if i == 0 {
			states[1].Value = domain.StateTrue
		}
		if err = r.ReplaceProjection(ctx, name, "coverage_object", id, states, nil); err != nil {
			t.Fatal(err)
		}
	}
	coverageQuery := domain.SubjectStateCoverageQuery{WorkspaceID: name, SubjectType: "coverage_object", AsOf: coverageAt, Watermark: time.Now(), StateKeys: []string{"X", "Y"}}
	coverage, err := r.Coverage(ctx, coverageQuery)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Total != 3 || coverage.Union != 2 || coverage.Intersection != 1 || len(coverage.States) != 2 || coverage.States[0].Satisfied != 2 || coverage.States[0].Unknown != 1 || coverage.States[1].Satisfied != 1 || coverage.States[1].False != 1 || coverage.States[1].Unknown != 1 {
		t.Fatal(coverage)
	}
	later := coverageQuery.Watermark.Add(time.Hour)
	if _, err = db.Exec(`INSERT INTO business_objects(id,object_type,customer_id,attributes,created_at) VALUES($1,'coverage_object',$2,'{}',$3)`, uuid.NewString(), customer, later); err != nil {
		t.Fatal(err)
	}
	if err = r.ReplaceProjection(ctx, name, "coverage_object", coverageIDs[2], []domain.SubjectStateInterval{{SubjectType: "coverage_object", SubjectID: coverageIDs[2], CustomerID: customer, StateKey: "X", Value: domain.StateTrue, ValidFrom: now, ComputedAt: later, RuleRevision: 1, Source: "rule"}}, nil); err != nil {
		t.Fatal(err)
	}
	repeated, err := r.Coverage(ctx, coverageQuery)
	if err != nil || repeated.Total != 3 || repeated.Union != 2 || repeated.States[0].Satisfied != 2 {
		t.Fatalf("future cohort/evidence leaked: %+v %v", repeated, err)
	}
	if _, err = db.Exec(`INSERT INTO customers(id,created_at,merged_into_id) VALUES($1,$2,$3)`, uuid.NewString(), now, customer); err != nil {
		t.Fatal(err)
	}
	canonicalQuery := domain.SubjectStateCoverageQuery{WorkspaceID: name, SubjectType: "customer", AsOf: time.Now(), Watermark: time.Now(), StateKeys: []string{"X"}}
	canonical, err := r.Coverage(ctx, canonicalQuery)
	if err != nil || canonical.Total != 1 || canonical.States[0].Unknown != 1 {
		t.Fatalf("merged source counted twice: %+v %v", canonical, err)
	}
	mergeApp, mergeOrder, targetCustomer := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for typ, id := range map[string]string{"merge_application": mergeApp, "merge_order": mergeOrder} {
		if _, err = db.Exec(`INSERT INTO business_objects(id,object_type,customer_id,attributes,created_at) VALUES($1,$2,$3,'{}',$4)`, id, typ, customer, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO business_definition_revisions VALUES($1,1,'{"kind":"object_relation","key":"customer_merge_applications","status":"active","settings":{"from_type":"customer","to_type":"merge_application"}}',clock_timestamp())`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO business_object_relations VALUES($1,'merge_application_orders',$2,$3,$4,NULL)`, uuid.NewString(), mergeApp, mergeOrder, now); err != nil {
		t.Fatal(err)
	}
	for i, code := range []string{"merge.click", "merge.apply", "merge.draw", "merge.pay"} {
		typ, id := "merge_order", mergeOrder
		if i == 0 {
			typ, id = "customer", customer
		}
		if i == 1 {
			typ, id = "merge_application", mergeApp
		}
		at := now.Add(time.Duration(i+1) * time.Second)
		if _, err = db.Exec(`INSERT INTO event_ledger(id,event_type,subject_type,subject_id,customer_id,occurred_at,received_at,properties) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp(),'{}')`, uuid.NewString(), code, typ, id, customer, at); err != nil {
			t.Fatal(err)
		}
	}
	beforeMerge := time.Now().UTC()
	mergedDefinition := domain.FunnelDefinition{SubjectType: "customer", WindowSeconds: 60, Steps: []domain.FunnelStep{{ID: "click", Events: []string{"merge.click"}}, {ID: "apply", SubjectType: "merge_application", RelationKey: "customer_merge_applications", Events: []string{"merge.apply"}}, {ID: "draw", SubjectType: "merge_order", RelationKey: "merge_application_orders", Events: []string{"merge.draw"}}, {ID: "pay", SubjectType: "merge_order", Events: []string{"merge.pay"}}}}
	checkMergedFunnel := func(watermark time.Time, wantCustomer string) {
		t.Helper()
		query := domain.FunnelQuery{Start: now, End: now.Add(time.Second * 2), Watermark: watermark}
		facts, readErr := NewFunnelRepositoryWithDB(db).Events(ctx, name, "customer", now, now.Add(time.Minute), watermark)
		if readErr != nil {
			t.Fatal(readErr)
		}
		query.Relations, readErr = NewFunnelRepositoryWithDB(db).ObjectRelations(ctx, name, now, now.Add(time.Minute), watermark)
		if readErr != nil {
			t.Fatal(readErr)
		}
		result, computeErr := domain.ComputeFunnel(mergedDefinition, query, facts, nil)
		if computeErr != nil || result.Converted != 1 || len(result.Details) != 1 || result.Details[0].CustomerID != wantCustomer {
			t.Fatalf("merge lost canonical linked conversion: %+v %v", result, computeErr)
		}
	}
	checkMergedFunnel(beforeMerge, customer)
	if _, err = db.Exec(`INSERT INTO customers(id) VALUES($1)`, targetCustomer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE customers SET merged_into_id=$1 WHERE id=$2`, targetCustomer, customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE business_objects SET customer_id=$1 WHERE customer_id=$2`, targetCustomer, customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE event_ledger SET customer_id=$1 WHERE customer_id=$2`, targetCustomer, customer); err != nil {
		t.Fatal(err)
	}
	checkMergedFunnel(beforeMerge, targetCustomer)
	checkMergedFunnel(time.Now().UTC(), targetCustomer)
	profileless := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO customers(id) VALUES($1)`, profileless); err != nil {
		t.Fatal(err)
	}
	profilelessContext, err := r.Context(ctx, name, "customer", profileless)
	if err != nil || len(profilelessContext.Attributes) != 0 {
		t.Fatalf("canonical customer without optional profile rejected: %+v %v", profilelessContext, err)
	}
	stateService := service.NewSubjectStateService(r, NewFunnelRepositoryWithDB(db), profilelessStateDefinitions{}, profilelessStateAuth{})
	if err = stateService.Set(ctx, service.SubjectStateSetRequest{WorkspaceID: name, SubjectType: "customer", SubjectID: profileless, Override: domain.StateOverride{StateKey: "manual_profileless", Value: domain.StateTrue, Reason: "Reviewed customer"}}); err != nil {
		t.Fatalf("manual state for profileless customer: %v", err)
	}
	manualStates, err := r.List(ctx, name, "customer", profileless, false)
	if err != nil || len(manualStates) != 1 || manualStates[0].Value != domain.StateTrue || manualStates[0].Source != "manual" {
		t.Fatalf("manual state not persisted: %+v %v", manualStates, err)
	}
	for _, absent := range []string{uuid.NewString(), customer} {
		if _, err = r.Context(ctx, name, "customer", absent); err == nil {
			t.Fatal("missing or merged customer accepted")
		}
	}
}

type profilelessStateAuth struct{ domain.AuthService }

func (profilelessStateAuth) AuthenticateUserForWorkspace(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	return context.WithValue(ctx, domain.UserIDKey, "state-test-actor"), &domain.User{}, &domain.UserWorkspace{Role: "owner"}, nil
}

type profilelessStateDefinitions struct {
	domain.BusinessDefinitionRepository
}

func (profilelessStateDefinitions) Get(context.Context, string, string, string) (*domain.BusinessDefinition, error) {
	return &domain.BusinessDefinition{Kind: "state", Key: "manual_profileless", Revision: 1, Status: "active", Settings: map[string]any{"subject_type": "customer", "rule": map[string]any{"kind": "event", "event": "manual.test.event"}}}, nil
}

func (d profilelessStateDefinitions) List(ctx context.Context, w string, _ domain.BusinessDefinitionFilter) ([]domain.BusinessDefinition, int, error) {
	definition, _ := d.Get(ctx, w, "state", "manual_profileless")
	return []domain.BusinessDefinition{*definition}, 1, nil
}
