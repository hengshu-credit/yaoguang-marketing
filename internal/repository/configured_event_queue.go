package repository

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *RealtimePostgresRepository) ClaimConfiguredEvents(ctx context.Context, w string, at time.Time, lease time.Duration, limit int) ([]domain.ConfiguredEventJob, error) {
	if limit < 1 || limit > 1000 || lease <= 0 {
		return nil, fmt.Errorf("invalid event job claim limits")
	}
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	token := uuid.New()
	rows, err := db.QueryContext(ctx, `WITH due AS(SELECT message_id FROM configured_event_jobs WHERE completed_at IS NULL AND available_at <= $1 AND (claim_until IS NULL OR claim_until <= $1) ORDER BY available_at,message_id LIMIT $2 FOR UPDATE SKIP LOCKED), claimed AS(UPDATE configured_event_jobs j SET claim_token=$3,claim_until=$4,attempts=attempts+1 FROM due WHERE j.message_id=due.message_id RETURNING j.message_id,j.claim_token) SELECT c.message_id,c.claim_token,o.payload FROM claimed c JOIN event_outbox o ON o.id=c.message_id`, at, limit, token, at.Add(lease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ConfiguredEventJob{}
	for rows.Next() {
		var job domain.ConfiguredEventJob
		if err := rows.Scan(&job.ID, &job.Token, &job.Payload); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (r *RealtimePostgresRepository) FinishConfiguredEvent(ctx context.Context, w string, job domain.ConfiguredEventJob, at time.Time, reason string) error {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE configured_event_jobs SET completed_at=CASE WHEN $4='' THEN $3::timestamptz ELSE NULL END,available_at=$3::timestamptz,last_error=$4,claim_until=NULL,claim_token=NULL WHERE message_id=$1 AND claim_token=$2 AND completed_at IS NULL`, job.ID, job.Token, at, reason)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("configured event job claim lost")
	}
	return nil
}
