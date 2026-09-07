package repository

import (
	"context"
	"database/sql"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type subjectStateTxKey struct{}
type subjectStateQuerier interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func subjectStateDB(ctx context.Context, base *RealtimePostgresRepository, w string) (subjectStateQuerier, error) {
	if tx, ok := ctx.Value(subjectStateTxKey{}).(*sql.Tx); ok {
		return tx, nil
	}
	return base.getDB(ctx, w)
}

// A transaction-level subject lock spans source reads and publication, so a slow
// projector cannot publish a stale read after a newer projection has committed.
func (r *SubjectStatePostgresRepository) WithSubjectLock(ctx context.Context, w, typ, id string, work func(context.Context) error) error {
	db, err := r.base.getDB(ctx, w)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, typ+":"+id); err != nil {
		return err
	}
	// Hold the dirty-marker row lock while reading. A concurrent ingestion can
	// append its event first, but its transactional dirty upsert waits and remains
	// scheduled after this projection commits; no generation can be acknowledged away.
	if _, err = tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id) VALUES($1,$2) ON CONFLICT(subject_type,subject_id) DO NOTHING`, typ, id); err != nil {
		return err
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM subject_state_projections WHERE subject_type=$1 AND subject_id=$2 FOR UPDATE`, typ, id).Scan(&revision); err != nil {
		return err
	}
	if err = work(context.WithValue(ctx, subjectStateTxKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

var _ domain.SubjectStateRepository = (*SubjectStatePostgresRepository)(nil)
