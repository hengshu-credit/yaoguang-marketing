package repository

import (
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionSnapshotSaveConflictingRunDoesNotOverwrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewExecutionSnapshotRepositoryWithDB(db)
	s, err := domain.NewExecutionSnapshot("campaign", "run-1", map[string]any{"name": "Original"}, nil)
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO execution_config_snapshots").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT content_hash").WithArgs("run-1").WillReturnRows(sqlmock.NewRows([]string{"content_hash"}).AddRow("different"))
	require.ErrorIs(t, repo.Save(context.Background(), "workspace-a", s), domain.ErrExecutionSnapshotConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExecutionSnapshotEvidenceRetryRejectsDifferentPayload(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewExecutionSnapshotRepositoryWithDB(db)
	evidence := domain.ExecutionEvidence{ExecutionID: "run-1", Key: "send-1", Kind: "delivery_content", Status: "rendered", Payload: json.RawMessage(`{"body":"original"}`)}
	mock.ExpectExec("INSERT INTO execution_evidence").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT payload_hash").WillReturnRows(sqlmock.NewRows([]string{"payload_hash"}).AddRow("different"))
	require.ErrorIs(t, repo.AppendEvidence(context.Background(), "ws", evidence), domain.ErrExecutionSnapshotConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
