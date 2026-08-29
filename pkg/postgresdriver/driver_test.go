package postgresdriver

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDriverIsRegistered(t *testing.T) {
	require.Contains(t, sql.Drivers(), Name)
}

func TestErrorDetailsSupportsPGXAndLegacyPQ(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "pgx",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "templates_pkey", Message: "duplicate"},
		},
		{
			name: "legacy pq",
			err:  &pq.Error{Code: "23505", Constraint: "templates_pkey", Message: "duplicate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details, ok := ErrorDetails(fmt.Errorf("wrapped: %w", tt.err))
			require.True(t, ok)
			require.Equal(t, "23505", details.Code)
			require.Equal(t, "templates_pkey", details.Constraint)
			require.Equal(t, "duplicate", details.Message)
		})
	}

	_, ok := ErrorDetails(errors.New("not postgres"))
	require.False(t, ok)
}
