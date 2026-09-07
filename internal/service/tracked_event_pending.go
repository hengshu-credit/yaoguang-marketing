package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
	"time"
)

type ResolvePendingEventRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Source      string `json:"source"`
	ReportID    string `json:"report_id"`
	CustomerID  string `json:"customer_id,omitempty"`
	Reason      string `json:"reason"`
}

func (s *TrackedEventService) ResolvePending(ctx context.Context, req ResolvePendingEventRequest) (*domain.TrackedEventResult, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return nil, err
	}
	_, _, member, err := s.auth.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if member == nil || !member.HasPermission(domain.PermissionResourceCustomers, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(domain.PermissionResourceCustomers, domain.PermissionTypeWrite, "Customer write access is required to resolve event identity")
	}
	if req.Source == "" || req.ReportID == "" || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		return nil, domain.NewValidationError("source,report_id and a resolution reason are required")
	}
	if req.CustomerID != "" {
		if _, err = uuid.Parse(req.CustomerID); err != nil {
			return nil, domain.NewValidationError("customer_id must be a UUID")
		}
	}
	resolver, ok := s.repo.(interface {
		ResolvePending(context.Context, string, string, string, string, string, time.Time) (*domain.TrackedEventResult, error)
	})
	if !ok {
		return nil, fmt.Errorf("pending event resolver unavailable")
	}
	return resolver.ResolvePending(ctx, req.WorkspaceID, req.Source, req.ReportID, req.CustomerID, strings.TrimSpace(req.Reason), s.now())
}
