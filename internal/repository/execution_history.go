package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *ExecutionSnapshotPostgresRepository) HistoryPage(ctx context.Context, ws, source, before string) (*domain.ExecutionHistoryPage, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+executionSnapshotColumns+` FROM execution_config_snapshots WHERE (source_id=$1 OR source_id IN(SELECT id::text FROM campaigns WHERE source_type='broadcast' AND source_id=$1)) AND ($2='' OR (created_at,id)<(SELECT created_at,id FROM execution_config_snapshots WHERE id=NULLIF($2,'')::uuid)) ORDER BY created_at DESC,id DESC LIMIT 51`, source, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &domain.ExecutionHistoryPage{Items: []domain.ExecutionSnapshot{}}
	for rows.Next() {
		snapshot, err := scanExecutionSnapshot(rows)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, *snapshot)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		page.Next = page.Items[49].ID
	}
	return page, nil
}
