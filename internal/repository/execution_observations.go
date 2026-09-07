package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *ExecutionSnapshotPostgresRepository) Observations(ctx context.Context, ws string, snapshot *domain.ExecutionSnapshot, now time.Time) (*domain.ExecutionObservations, error) {
	plan, err := domain.ExecutionTrackingPlan(snapshot.Configuration)
	if err != nil {
		return nil, err
	}
	empty := &domain.ExecutionObservations{ExecutionID: snapshot.ExecutionID, Watermark: now, Events: []domain.ExecutionObservationMetric{}, States: []domain.ExecutionObservationMetric{}}
	if plan == nil {
		return empty, nil
	}
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	funnel := NewFunnelRepositoryWithDB(db)
	rows, err := db.QueryContext(ctx, `SELECT customer_id::text,'' AS subject_type,'' AS subject_id,created_at FROM campaign_recipient_snapshots WHERE run_id::text=$1 UNION ALL SELECT e.customer_id::text,COALESCE(s.subject_type,''),COALESCE(s.subject_id,''),i.started_at FROM journey_instances i JOIN journey_enrollments e ON e.id=i.enrollment_id LEFT JOIN journey_event_subjects s ON s.contact_automation_id=i.contact_automation_id WHERE i.id::text=$1`, snapshot.ExecutionID)
	if err != nil {
		return nil, err
	}
	cohort := []domain.ExecutionObservationSubject{}
	for rows.Next() {
		var c domain.ExecutionObservationSubject
		if err = rows.Scan(&c.CustomerID, &c.SubjectType, &c.SubjectID, &c.Since); err != nil {
			rows.Close()
			return nil, err
		}
		cohort = append(cohort, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	end := now
	deadline := snapshot.CreatedAt.Add(time.Duration(plan.WindowSeconds) * time.Second)
	for _, c := range cohort {
		candidate := c.Since.Add(time.Duration(plan.WindowSeconds) * time.Second)
		if candidate.After(deadline) {
			deadline = candidate
		}
	}
	if end.After(deadline) {
		end = deadline
	}
	events, err := funnel.Events(ctx, ws, plan.SubjectType, snapshot.CreatedAt, end, now)
	if err != nil {
		return nil, err
	}
	links, err := funnel.AttributionLinks(ctx, ws, snapshot.CreatedAt, end, now)
	if err != nil {
		return nil, err
	}
	states, err := funnel.StateHistory(ctx, ws, snapshot.CreatedAt, end)
	if err != nil {
		return nil, err
	}
	result := domain.SummarizeExecutionObservations(snapshot.ExecutionID, *plan, cohort, events, states, links, now)
	return &result, nil
}
