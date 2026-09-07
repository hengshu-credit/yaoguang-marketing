package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
	"time"
)

func TestSubjectStateCoverageRejectsPermissionScopeAndFuture(t *testing.T) {
	denied := NewSubjectStateService(nil, nil, nil, businessTestAuth{member: &domain.UserWorkspace{Role: "member"}})
	if _, err := denied.Coverage(context.Background(), domain.SubjectStateCoverageQuery{WorkspaceID: "w"}); err == nil {
		t.Fatal("customer read permission required")
	}
	defs := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"state:X": {Kind: "state", Key: "X", Settings: map[string]any{"subject_type": "other"}}}}
	allowed := NewSubjectStateService(nil, nil, defs, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	q := domain.SubjectStateCoverageQuery{WorkspaceID: "w", SubjectType: "customer", AsOf: time.Now(), Watermark: time.Now(), StateKeys: []string{"X"}}
	if _, err := allowed.Coverage(context.Background(), q); err == nil {
		t.Fatal("wrong state scope accepted")
	}
	q.Watermark = time.Now().Add(time.Hour)
	if _, err := allowed.Coverage(context.Background(), q); err == nil {
		t.Fatal("future watermark accepted")
	}
}
