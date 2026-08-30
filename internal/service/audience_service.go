package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type AudienceService struct {
	repository domain.AudienceRepository
	auth       *AuthService
	tasks      ImportTaskScheduler
	builds     AudienceBuildRunner
}

type AudienceBuildRunner interface {
	StartAudienceBuild(context.Context, string, string, int) (*domain.AudienceBuild, error)
	ProcessAudienceBuildChunk(context.Context, string, string, int) (*domain.AudienceBuild, bool, error)
}

func NewAudienceService(repository domain.AudienceRepository) (*AudienceService, error) {
	if repository == nil {
		return nil, errors.New("audience repository is required")
	}
	result := &AudienceService{repository: repository}
	if builds, ok := repository.(AudienceBuildRunner); ok {
		result.builds = builds
	}
	return result, nil
}

func (s *AudienceService) SetTaskScheduler(tasks ImportTaskScheduler) {
	s.tasks = tasks
}

func NewAuthorizedAudienceService(repository domain.AudienceRepository, auth *AuthService) (*AudienceService, error) {
	result, err := NewAudienceService(repository)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, errors.New("audience auth service is required")
	}
	result.auth = auth
	return result, nil
}

func (s *AudienceService) authorize(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	if s.auth == nil {
		return ctx, nil
	}
	authorized, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, err
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceSegments, permission) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceSegments, permission, "Insufficient permissions")
	}
	return authorized, nil
}

type CreateAudienceRequest struct {
	WorkspaceID string
	Name        string
	Description string
	Kind        domain.AudienceKind
	Definition  domain.AudienceExpression
}

func (s *AudienceService) Create(ctx context.Context, request CreateAudienceRequest) (*domain.Audience, error) {
	if strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("workspace and audience name are required")
	}
	if err := request.Definition.Validate(); err != nil {
		return nil, err
	}
	authorized, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	audience := domain.Audience{ID: uuid.New().String(), Name: strings.TrimSpace(request.Name), Description: strings.TrimSpace(request.Description),
		Kind: request.Kind, ActiveVersion: 1, CreatedAt: now, UpdatedAt: now}
	if audience.Kind == "" {
		audience.Kind = domain.AudienceKindDynamic
	}
	if err := s.validateDependencyGraph(authorized, request.WorkspaceID, audience.ID, request.Definition); err != nil {
		return nil, err
	}
	hash, _ := request.Definition.VersionHash()
	version := domain.AudienceVersion{AudienceID: audience.ID, Version: 1, Definition: request.Definition, DefinitionHash: hash, CreatedAt: now}
	if err := s.repository.CreateAudience(authorized, request.WorkspaceID, audience, version); err != nil {
		return nil, err
	}
	return &audience, nil
}

func (s *AudienceService) UpdateDefinition(ctx context.Context, workspaceID, audienceID string, expression domain.AudienceExpression) (*domain.AudienceVersion, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	if containsAudienceReference(expression, audienceID) {
		return nil, errors.New("audience cannot reference itself")
	}
	if err := s.validateDependencyGraph(authorized, workspaceID, audienceID, expression); err != nil {
		return nil, err
	}
	return s.repository.SaveAudienceVersion(authorized, workspaceID, audienceID, expression)
}

func (s *AudienceService) Preview(ctx context.Context, workspaceID string, expression domain.AudienceExpression) ([]domain.CustomerSummary, int64, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.PreviewAudience(authorized, workspaceID, expression, 100)
}

func (s *AudienceService) Build(ctx context.Context, workspaceID, audienceID string, version int) (string, int64, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return "", 0, err
	}
	if s.tasks == nil || s.builds == nil {
		return s.repository.BuildAudience(authorized, workspaceID, audienceID, version)
	}
	build, err := s.builds.StartAudienceBuild(authorized, workspaceID, audienceID, version)
	if err != nil {
		return "", 0, err
	}
	task := &domain.Task{ID: build.ID, WorkspaceID: workspaceID, Type: domain.BuildAudienceTaskType,
		Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 20, RetryInterval: 10,
		State: &domain.TaskState{BuildAudience: &domain.BuildAudienceState{BuildID: build.ID}}}
	if err := s.tasks.CreateTask(authorized, workspaceID, task); err != nil {
		return "", 0, fmt.Errorf("create audience build task: %w", err)
	}
	return build.ID, 0, nil
}

func (s *AudienceService) Get(ctx context.Context, workspaceID, audienceID string) (*domain.Audience, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.GetAudience(authorized, workspaceID, audienceID)
}

func (s *AudienceService) List(ctx context.Context, workspaceID string, limit, offset int) ([]domain.Audience, int, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.ListAudiences(authorized, workspaceID, limit, offset)
}

func (s *AudienceService) BuildStatus(ctx context.Context, workspaceID, buildID string) (*domain.AudienceBuild, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.GetAudienceBuild(authorized, workspaceID, buildID)
}

func (s *AudienceService) Members(ctx context.Context, workspaceID, buildID, after string, limit int) ([]domain.CustomerSummary, string, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, "", err
	}
	return s.repository.ListAudienceMembers(authorized, workspaceID, buildID, after, limit)
}

func (s *AudienceService) Delete(ctx context.Context, workspaceID, audienceID string) error {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	return s.repository.ArchiveAudience(authorized, workspaceID, audienceID)
}

func (s *AudienceService) validateDependencyGraph(ctx context.Context, workspaceID, rootID string, expression domain.AudienceExpression) error {
	visiting := map[string]bool{rootID: true}
	visited := map[string]bool{}
	var walkExpression func(domain.AudienceExpression) error
	var walkAudience func(string) error
	walkAudience = func(audienceID string) error {
		if visiting[audienceID] {
			return fmt.Errorf("audience dependency cycle includes %s", audienceID)
		}
		if visited[audienceID] {
			return nil
		}
		item, err := s.repository.GetAudience(ctx, workspaceID, audienceID)
		if err != nil {
			return fmt.Errorf("referenced audience %s is unavailable: %w", audienceID, err)
		}
		version, err := s.repository.GetAudienceVersion(ctx, workspaceID, item.ID, item.ActiveVersion)
		if err != nil {
			return fmt.Errorf("load referenced audience %s version: %w", audienceID, err)
		}
		visiting[audienceID] = true
		if err := walkExpression(version.Definition); err != nil {
			return err
		}
		delete(visiting, audienceID)
		visited[audienceID] = true
		return nil
	}
	walkExpression = func(item domain.AudienceExpression) error {
		if item.LeafType == domain.AudienceLeafAudience {
			return walkAudience(item.RefID)
		}
		for _, child := range item.Children {
			if err := walkExpression(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walkExpression(expression)
}

func containsAudienceReference(expression domain.AudienceExpression, audienceID string) bool {
	if expression.LeafType == domain.AudienceLeafAudience && expression.RefID == audienceID {
		return true
	}
	for _, child := range expression.Children {
		if containsAudienceReference(child, audienceID) {
			return true
		}
	}
	return false
}
