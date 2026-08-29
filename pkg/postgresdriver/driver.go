// Package postgresdriver owns the database/sql PostgreSQL driver used by the
// application. pgx uses stable named prepared statements, which PgBouncer can
// safely track across server connections in transaction-pooling mode.
package postgresdriver

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"
)

// Name is the database/sql driver registered by pgx/v5/stdlib.
const Name = "pgx"

// Details is the portable subset of PostgreSQL errors used by repositories.
// Legacy pq support is retained for sqlmock tests and embedders that still
// return pq.Error while the application runtime uses pgx.
type Details struct {
	Code       string
	Constraint string
	Message    string
}

// ErrorDetails extracts PostgreSQL diagnostics from pgx or legacy lib/pq.
func ErrorDetails(err error) (Details, bool) {
	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		return Details{
			Code:       pgxErr.Code,
			Constraint: pgxErr.ConstraintName,
			Message:    pgxErr.Message,
		}, true
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return Details{
			Code:       string(pqErr.Code),
			Constraint: pqErr.Constraint,
			Message:    pqErr.Message,
		}, true
	}

	return Details{}, false
}
