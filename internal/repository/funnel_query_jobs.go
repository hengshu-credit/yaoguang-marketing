package repository

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *FunnelPostgresRepository) CreateQuery(ctx context.Context, w string, job *domain.FunnelQueryJob) error {
	db, err := r.base.getDB(ctx, w)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(job.Query)
	if err != nil {
		return err
	}
	return db.QueryRowContext(ctx, `INSERT INTO funnel_query_jobs(id,status,query) VALUES($1,'running',$2) RETURNING created_at,updated_at`, job.ID, raw).Scan(&job.CreatedAt, &job.UpdatedAt)
}
func (r *FunnelPostgresRepository) GetQuery(ctx context.Context, w, id string) (*domain.FunnelQueryJob, error) {
	db, err := r.base.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	var job domain.FunnelQueryJob
	var query, result []byte
	err = db.QueryRowContext(ctx, `SELECT id::text,status,query,result,error,created_at,updated_at FROM funnel_query_jobs WHERE id=$1`, id).Scan(&job.ID, &job.Status, &query, &result, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(query, &job.Query); err != nil {
		return nil, err
	}
	if len(result) > 0 && string(result) != "null" {
		if err = json.Unmarshal(result, &job.Result); err != nil {
			return nil, err
		}
	}
	return &job, nil
}
func (r *FunnelPostgresRepository) FinishQuery(ctx context.Context, w, id, status string, result *domain.FunnelResult, message string) error {
	db, err := r.base.getDB(ctx, w)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE funnel_query_jobs SET status=$2,result=$3,error=$4,updated_at=NOW() WHERE id=$1 AND status IN ('queued','running')`, id, status, raw, message)
	return err
}
func (r *FunnelClickHouseRepository) CreateQuery(ctx context.Context, w string, j *domain.FunnelQueryJob) error {
	return r.source.CreateQuery(ctx, w, j)
}
func (r *FunnelClickHouseRepository) GetQuery(ctx context.Context, w, id string) (*domain.FunnelQueryJob, error) {
	return r.source.GetQuery(ctx, w, id)
}
func (r *FunnelClickHouseRepository) FinishQuery(ctx context.Context, w, id, status string, result *domain.FunnelResult, message string) error {
	return r.source.FinishQuery(ctx, w, id, status, result, message)
}
