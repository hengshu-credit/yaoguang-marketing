package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (s *FunnelService) StartQuery(ctx context.Context, q domain.FunnelQuery) (*domain.FunnelQueryJob, error) {
	ctx, err := authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceWebAnalytics, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	if q.IncludeDetails {
		if _, err = authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceCustomers, domain.PermissionTypeRead); err != nil {
			return nil, err
		}
	}
	repo, ok := s.repository.(domain.FunnelQueryJobRepository)
	if !ok {
		return nil, domain.NewValidationError("asynchronous query storage unavailable")
	}
	if q.Definition == nil {
		record, err := s.definitions.Get(ctx, q.WorkspaceID, "funnel", q.Key)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(record.Settings)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &q.Definition); err != nil {
			return nil, err
		}
	}
	if q.Definition == nil {
		return nil, domain.NewValidationError("funnel definition required")
	}
	if err = q.Definition.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	if q.Watermark.IsZero() {
		q.Watermark = time.Now().UTC()
	}
	job := &domain.FunnelQueryJob{ID: uuid.NewString(), Status: "running", Query: q}
	if err = repo.CreateQuery(ctx, q.WorkspaceID, job); err != nil {
		return nil, err
	}
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	s.jobsMu.Lock()
	s.jobs[job.ID] = cancel
	s.jobsMu.Unlock()
	go func() {
		defer cancel()
		defer func() { s.jobsMu.Lock(); delete(s.jobs, job.ID); s.jobsMu.Unlock() }()
		select {
		case s.querySlots <- struct{}{}:
			defer func() { <-s.querySlots }()
		case <-work.Done():
			_ = repo.FinishQuery(context.Background(), q.WorkspaceID, job.ID, "failed", nil, "Query timed out before execution")
			return
		}
		result, err := s.Query(work, q)
		status, message := "completed", ""
		if err != nil {
			status = "failed"
			message = "Query failed or exceeded its time limit"
		}
		_ = repo.FinishQuery(context.Background(), q.WorkspaceID, job.ID, status, &result, message)
	}()
	return job, nil
}
func (s *FunnelService) GetQuery(ctx context.Context, w, id string) (*domain.FunnelQueryJob, error) {
	ctx, err := authorizeAnalysis(ctx, s.auth, w, domain.PermissionResourceWebAnalytics, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, domain.NewValidationError("invalid query ID")
	}
	repo, ok := s.repository.(domain.FunnelQueryJobRepository)
	if !ok {
		return nil, fmt.Errorf("query storage unavailable")
	}
	job, err := repo.GetQuery(ctx, w, id)
	if err != nil {
		return nil, err
	}
	if job.Query.IncludeDetails {
		if _, err = authorizeAnalysis(ctx, s.auth, w, domain.PermissionResourceCustomers, domain.PermissionTypeRead); err != nil {
			return nil, err
		}
	}
	if (job.Status == "running" || job.Status == "queued") && time.Since(job.CreatedAt) > 3*time.Minute {
		if err = repo.FinishQuery(ctx, w, id, "failed", nil, "Query execution was interrupted"); err != nil {
			return nil, err
		}
		job.Status = "failed"
		job.Error = "Query execution was interrupted"
	}
	return job, nil
}
func (s *FunnelService) CancelQuery(ctx context.Context, w, id string) error {
	if _, err := s.GetQuery(ctx, w, id); err != nil {
		return err
	}
	repo := s.repository.(domain.FunnelQueryJobRepository)
	if err := repo.FinishQuery(ctx, w, id, "cancelled", nil, ""); err != nil {
		return err
	}
	s.jobsMu.Lock()
	cancel := s.jobs[id]
	s.jobsMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
