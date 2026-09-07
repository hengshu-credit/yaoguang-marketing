package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
	"time"
)

func TestFunnelRepositoryUsesWorkspaceLedgerWatermark(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewFunnelRepositoryWithDB(db)
	start := time.Now().UTC()
	end := start.Add(time.Hour)
	watermark := end.Add(time.Hour)
	m.ExpectQuery("SELECT.*event_type.*FROM event_ledger").WithArgs(start, end, watermark, "contract", 100001).WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "subject_type", "subject_id", "customer_id", "occurred_at", "received_at", "sequence", "properties"}).AddRow("e", "signed", "contract", "A", "c", start, start, nil, `{"data":{"amount_minor":1250,"currency":"CNY","transaction_id":"p"}}`))
	events, err := r.Events(context.Background(), "w", "contract", start, end, watermark)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].AmountMinor != 1250 || events[0].Properties["currency"] != "CNY" {
		t.Fatal(events)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSubjectStateRepositoryRejectsUnauditedOverride(t *testing.T) {
	r := NewSubjectStateRepositoryWithDB(nil)
	err := r.SetOverride(context.Background(), "w", "contract", "A", domain.StateOverride{StateKey: "eligible", Value: domain.StateTrue})
	if err == nil {
		t.Fatal("reason actor and validity required before DB access")
	}
}

func TestSubjectStateRepositoryLocksBeforeProjectionSourceReads(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewSubjectStateRepositoryWithDB(db)
	m.ExpectBegin()
	m.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("contract:A").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO subject_state_projections").WithArgs("contract", "A").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT revision.*FOR UPDATE").WithArgs("contract", "A").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(0))
	m.ExpectQuery("SELECT state_key").WithArgs("contract", "A").WillReturnRows(sqlmock.NewRows([]string{"state_key", "value", "reason", "actor", "valid_from", "valid_to"}))
	m.ExpectCommit()
	err = r.WithSubjectLock(context.Background(), "w", "contract", "A", func(ctx context.Context) error { _, err := r.Overrides(ctx, "w", "contract", "A"); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
