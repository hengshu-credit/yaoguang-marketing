package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (s *SubjectStateService) Coverage(ctx context.Context, q domain.SubjectStateCoverageQuery) (domain.SubjectStateCoverage, error) {
	empty := domain.SubjectStateCoverage{}
	ctx, err := authorizeAnalysis(ctx, s.auth, q.WorkspaceID, domain.PermissionResourceCustomers, domain.PermissionTypeRead)
	if err != nil {
		return empty, err
	}
	now := time.Now().UTC()
	if q.AsOf.IsZero() {
		q.AsOf = now
	}
	if q.Watermark.IsZero() {
		q.Watermark = now
	}
	if q.SubjectType == "" || len(q.StateKeys) < 1 || len(q.StateKeys) > 50 || q.AsOf.After(q.Watermark) || q.Watermark.After(now.Add(time.Minute)) {
		return empty, domain.NewValidationError("subject_type, one to fifty states, and as_of <= watermark <= now are required")
	}
	seen := map[string]bool{}
	for _, key := range q.StateKeys {
		if seen[key] {
			return empty, domain.NewValidationError("selected state keys must be distinct")
		}
		seen[key] = true
		d, err := s.definitions.Get(ctx, q.WorkspaceID, "state", key)
		if err != nil {
			return empty, domain.NewValidationError("selected state not found")
		}
		if d.Settings["subject_type"] != q.SubjectType {
			return empty, domain.NewValidationError("state subject type does not match")
		}
	}
	reader, ok := s.repository.(interface {
		Coverage(context.Context, domain.SubjectStateCoverageQuery) (domain.SubjectStateCoverage, error)
	})
	if !ok {
		return empty, domain.NewValidationError("state coverage reader unavailable")
	}
	limited, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return reader.Coverage(limited, q)
}
