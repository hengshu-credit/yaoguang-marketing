package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestFunnelQueryCancellationPreventsLaterCompletion(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewFunnelRepositoryWithDB(db)
	m.ExpectExec("UPDATE funnel_query_jobs.*status IN.*queued.*running").WithArgs("job", "cancelled", sqlmock.AnyArg(), "").WillReturnResult(sqlmock.NewResult(0, 1))
	if err = r.FinishQuery(context.Background(), "w", "job", "cancelled", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
