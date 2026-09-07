package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
)

func TestFunnelAttributionRepositoryUsesCapturedDeliveryEvidence(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewFunnelRepositoryWithDB(db)
	start := time.Now().UTC()
	end := start.Add(time.Hour)
	m.ExpectQuery("WITH delivered AS.*execution_evidence.*event_ledger").WithArgs(start, end, end).WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "customer_id", "kind", "at"}).AddRow("message-1", "run-1", "customer-1", "click", start))
	links, err := r.AttributionLinks(context.Background(), "w", start, end, end)
	if err != nil || len(links) != 1 || links[0].RunID != "run-1" {
		t.Fatalf("%+v %v", links, err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
