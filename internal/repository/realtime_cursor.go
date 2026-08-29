package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type WorkspaceCursorPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewWorkspaceCursorRepository(workspaceRepo domain.WorkspaceRepository) domain.WorkspaceCursorRepository {
	return &WorkspaceCursorPostgresRepository{workspaceRepo: workspaceRepo}
}

func NewWorkspaceCursorRepositoryWithDB(db *sql.DB) domain.WorkspaceCursorRepository {
	return &WorkspaceCursorPostgresRepository{db: db}
}

func (r *WorkspaceCursorPostgresRepository) NextWorkspaceIDs(
	ctx context.Context,
	cursorName string,
	limit int,
) ([]string, error) {
	if cursorName == "" || limit <= 0 {
		return nil, errors.New("cursor name and positive limit are required")
	}
	db, err := r.systemDB(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin workspace cursor transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO realtime_runtime_cursors (name)
		VALUES ($1)
		ON CONFLICT (name) DO NOTHING
	`, cursorName); err != nil {
		return nil, fmt.Errorf("initialize workspace cursor: %w", err)
	}

	var lastWorkspaceID sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT last_workspace_id
		FROM realtime_runtime_cursors
		WHERE name = $1
		FOR UPDATE
	`, cursorName).Scan(&lastWorkspaceID); err != nil {
		return nil, fmt.Errorf("lock workspace cursor: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM workspaces
		ORDER BY CASE WHEN id > $1 THEN 0 ELSE 1 END, id
		LIMIT $2
	`, lastWorkspaceID.String, limit)
	if err != nil {
		return nil, fmt.Errorf("list workspaces from cursor: %w", err)
	}
	var workspaceIDs []string
	for rows.Next() {
		var workspaceID string
		if err := rows.Scan(&workspaceID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan cursor workspace: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close cursor workspace rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cursor workspaces: %w", err)
	}

	if len(workspaceIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE realtime_runtime_cursors
			SET last_workspace_id = $2, updated_at = CURRENT_TIMESTAMP
			WHERE name = $1
		`, cursorName, workspaceIDs[len(workspaceIDs)-1]); err != nil {
			return nil, fmt.Errorf("advance workspace cursor: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit workspace cursor: %w", err)
	}
	return workspaceIDs, nil
}

func (r *WorkspaceCursorPostgresRepository) systemDB(ctx context.Context) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace cursor has no system database")
	}
	db, err := r.workspaceRepo.GetSystemConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system connection for workspace cursor: %w", err)
	}
	return db, nil
}
