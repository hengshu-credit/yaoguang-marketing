package service

import (
	"context"
	"errors"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AutomationAudienceRunRepository interface {
	GetByID(context.Context, string, string) (*domain.Automation, error)
	GetAutomationRuntimeVersion(context.Context, string, string) (int, error)
	EnrollAudienceBuild(context.Context, string, string, string, int, string, int, string) (int64, error)
}

type AutomationAudienceRunRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	AutomationID string `json:"automation_id"`
	AudienceID   string `json:"audience_id"`
}

type AutomationAudienceRunResult struct {
	AutomationID    string `json:"automation_id"`
	AudienceID      string `json:"audience_id"`
	AudienceVersion int    `json:"audience_version"`
	BuildID         string `json:"build_id"`
	CandidateCount  int64  `json:"candidate_count"`
	EnrolledCount   int64  `json:"enrolled_count"`
}

type AutomationAudienceRunService struct {
	repository AutomationAudienceRunRepository
	audiences  AudienceExecutionService
	auth       *AuthService
}

func NewAutomationAudienceRunService(repository AutomationAudienceRunRepository, audiences AudienceExecutionService, auth *AuthService) (*AutomationAudienceRunService, error) {
	if repository == nil || audiences == nil {
		return nil, errors.New("automation audience repository and audience execution service are required")
	}
	return &AutomationAudienceRunService{repository: repository, audiences: audiences, auth: auth}, nil
}

func (s *AutomationAudienceRunService) Start(ctx context.Context, request AutomationAudienceRunRequest) (*AutomationAudienceRunResult, error) {
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.AutomationID = strings.TrimSpace(request.AutomationID)
	request.AudienceID = strings.TrimSpace(request.AudienceID)
	if request.WorkspaceID == "" || request.AutomationID == "" || request.AudienceID == "" {
		return nil, domain.NewValidationError("workspace_id, automation_id, and audience_id are required")
	}
	if s.auth != nil {
		authorized, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if membership == nil || !membership.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeWrite) {
			return nil, domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "Insufficient permissions")
		}
		if !membership.HasPermission(domain.PermissionResourceSegments, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(domain.PermissionResourceSegments, domain.PermissionTypeRead, "Insufficient permissions")
		}
		if !membership.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Insufficient permissions")
		}
		ctx = authorized
	}

	automation, err := s.repository.GetByID(ctx, request.WorkspaceID, request.AutomationID)
	if err != nil {
		return nil, err
	}
	if automation.Status != domain.AutomationStatusLive {
		return nil, domain.NewValidationError("automation must be live before starting an audience run")
	}
	if automation.RootNodeID == "" || automation.GetNodeByID(automation.RootNodeID) == nil {
		return nil, domain.NewValidationError("automation root node is unavailable")
	}
	automationVersion, err := s.repository.GetAutomationRuntimeVersion(ctx, request.WorkspaceID, automation.ID)
	if err != nil {
		return nil, err
	}
	build, err := s.audiences.ResolveLatestAndBuildInternal(ctx, request.WorkspaceID, request.AudienceID)
	if err != nil {
		return nil, err
	}
	if build.Status != "completed" {
		return nil, domain.NewValidationError("audience candidate snapshot did not complete")
	}
	enrolled, err := s.repository.EnrollAudienceBuild(
		ctx, request.WorkspaceID, automation.ID, automation.RootNodeID, automationVersion,
		build.AudienceID, build.AudienceVersion, build.ID,
	)
	if err != nil {
		return nil, err
	}
	return &AutomationAudienceRunResult{
		AutomationID: automation.ID, AudienceID: build.AudienceID, AudienceVersion: build.AudienceVersion,
		BuildID: build.ID, CandidateCount: build.MemberCount, EnrolledCount: enrolled,
	}, nil
}
