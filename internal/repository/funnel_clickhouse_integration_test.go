//go:build integration

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/eventanalytics"
	"os"
	"testing"
	"time"
)

func TestFunnelPostgresClickHouseParity(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "true" || os.Getenv("TEST_CLICKHOUSE_ADDR") == "" {
		t.Skip("requires isolated PostgreSQL and ClickHouse")
	}
	ctx := context.Background()
	name := "analysis_parity_" + fmt.Sprint(time.Now().UnixNano())
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "notifuse-configurable-postgres"
	}
	dsn := fmt.Sprintf("host=%s port=5432 user=notifuse_test password=test_password sslmode=disable", host)
	admin, err := sql.Open("postgres", dsn+" dbname=postgres")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatal(err)
	}
	pg, err := sql.Open("postgres", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pg.Close()
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	}()
	if _, err = pg.Exec(`CREATE TABLE event_ledger(id UUID PRIMARY KEY,event_type TEXT,subject_type TEXT,subject_id TEXT,customer_id UUID,occurred_at TIMESTAMPTZ,received_at TIMESTAMPTZ,sequence BIGINT,properties JSONB)`); err != nil {
		t.Fatal(err)
	}
	connection, err := clickhouse.Open(&clickhouse.Options{Addr: []string{os.Getenv("TEST_CLICKHOUSE_ADDR")}, Auth: clickhouse.Auth{Username: "notifuse_test", Password: "test_password"}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	store, err := eventanalytics.NewClickHouseStore(connection, name)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := connection.Exec(ctx, "DROP DATABASE "+name); err != nil {
			t.Error(err)
		}
	}()
	now := time.Now().UTC().Truncate(time.Second)
	base := now.Add(-3 * time.Hour)
	customer := uuid.NewString()
	input := []struct {
		subject, code string
		seconds       int
	}{{"A", "start", 0}, {"B", "finish", 10}, {"C", "start", 0}, {"C", "start", 7200}, {"C", "finish", 7210}}
	envelopes := []domain.EventEnvelope{}
	for _, item := range input {
		at := base.Add(time.Duration(item.seconds) * time.Second)
		id := uuid.New()
		e := domain.EventEnvelope{ID: uuid.New(), EventID: id, WorkspaceID: name, Type: item.code, SchemaVersion: 1, Subject: domain.EventSubject{Type: "contract", ID: item.subject, CustomerID: customer}, Source: "integration", OccurredAt: at, ReceivedAt: at, CorrelationID: id, Data: json.RawMessage(`{"data":{}}`)}
		if _, err = pg.Exec(`INSERT INTO event_ledger VALUES($1,$2,$3,$4,$5,$6,$6,NULL,$7)`, e.EventID, e.Type, e.Subject.Type, e.Subject.ID, customer, at, []byte(e.Data)); err != nil {
			t.Fatal(err)
		}
		envelopes = append(envelopes, e)
	}
	if err = store.InsertBatch(ctx, append(envelopes, envelopes...)); err != nil {
		t.Fatal(err)
	}
	pgr := NewFunnelRepositoryWithDB(pg)
	q := domain.FunnelQuery{Start: base, End: base.Add(8000 * time.Second), Watermark: now}
	definition := domain.FunnelDefinition{SubjectType: "contract", WindowSeconds: 3600, Steps: []domain.FunnelStep{{ID: "start", Events: []string{"start"}}, {ID: "finish", Events: []string{"finish"}}}}
	pgEvents, err := pgr.Events(ctx, name, "contract", base, now, now)
	if err != nil {
		t.Fatal(err)
	}
	chEvents, err := store.Events(ctx, name, "contract", base, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(pgEvents) != 5 || len(chEvents) != 5 {
		t.Fatalf("duplicate projection rows visible: PG %d CH %d", len(pgEvents), len(chEvents))
	}
	pResult, err := domain.ComputeFunnel(definition, q, pgEvents, nil)
	if err != nil {
		t.Fatal(err)
	}
	cResult, err := domain.ComputeFunnel(definition, q, chEvents, nil)
	if err != nil {
		t.Fatal(err)
	}
	pResult.ComputedAt = time.Time{}
	cResult.ComputedAt = time.Time{}
	pJSON, _ := json.Marshal(pResult)
	cJSON, _ := json.Marshal(cResult)
	if !bytes.Equal(pJSON, cJSON) {
		t.Fatalf("PG/CH mismatch\nPG=%+v\nCH=%+v", pResult, cResult)
	}
	if pResult.Entered != 2 || pResult.Converted != 1 {
		t.Fatal(pResult)
	}
}
