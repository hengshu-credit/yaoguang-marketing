package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type CampaignService struct {
	repository domain.CampaignRepository
	snapshots  *CampaignSnapshotService
	auth       *AuthService
	tasks      ImportTaskScheduler
	audiences  AudienceExecutionService
}

type AudienceExecutionService interface {
	ResolveLatestAndBuildInternal(context.Context, string, string) (*domain.AudienceBuild, error)
}

type broadcastCampaignRepository interface {
	EnsureBroadcastCampaign(context.Context, string, string, string, string, int, string, string, []domain.CampaignVariant) (*domain.CampaignVersion, error)
}

func NewCampaignService(repository domain.CampaignRepository, snapshots *CampaignSnapshotService) (*CampaignService, error) {
	if repository == nil || snapshots == nil {
		return nil, errors.New("campaign repository and snapshot service are required")
	}
	return &CampaignService{repository: repository, snapshots: snapshots}, nil
}

func NewAuthorizedCampaignService(repository domain.CampaignRepository, snapshots *CampaignSnapshotService, auth *AuthService) (*CampaignService, error) {
	result, err := NewCampaignService(repository, snapshots)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, errors.New("campaign auth service is required")
	}
	result.auth = auth
	return result, nil
}

func (s *CampaignService) SetTaskScheduler(tasks ImportTaskScheduler) {
	s.tasks = tasks
}

func (s *CampaignService) SetAudienceExecutionService(audiences AudienceExecutionService) {
	s.audiences = audiences
}

func (s *CampaignService) authorize(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	if s.auth == nil {
		return ctx, nil
	}
	authorized, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, err
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceBroadcasts, permission) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceBroadcasts, permission, "Insufficient permissions")
	}
	return authorized, nil
}

type CreateCampaignRequest struct {
	WorkspaceID     string
	Name            string
	AudienceID      string
	AudienceVersion int
	ListID          string
	Channel         string
	Variants        []domain.CampaignVariant
}

func (s *CampaignService) Create(ctx context.Context, request CreateCampaignRequest) (*domain.Campaign, error) {
	if strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("workspace and campaign name are required")
	}
	authorized, err := s.authorize(ctx, request.WorkspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	campaign := domain.Campaign{ID: uuid.New().String(), Name: strings.TrimSpace(request.Name), Status: domain.CampaignStatusDraft,
		DraftVersion: 1, CreatedAt: now, UpdatedAt: now}
	version := domain.CampaignVersion{CampaignID: campaign.ID, Version: 1, AudienceID: request.AudienceID,
		AudienceVersion: request.AudienceVersion, ListID: request.ListID, Channel: request.Channel, Variants: request.Variants, CreatedAt: now}
	if err := version.Validate(); err != nil {
		return nil, err
	}
	if current, ok := s.repository.(currentCampaignCreator); ok {
		config := domain.CampaignConfiguration{AudienceID: version.AudienceID, AudienceVersion: version.AudienceVersion, ListID: version.ListID, Channel: version.Channel, Variants: version.Variants}
		if err := current.CreateCurrentCampaign(authorized, request.WorkspaceID, campaign, config); err != nil {
			return nil, err
		}
		return &campaign, nil
	}
	if err := s.repository.CreateCampaign(authorized, request.WorkspaceID, campaign, version); err != nil {
		return nil, err
	}
	return &campaign, nil
}

func (s *CampaignService) List(ctx context.Context, workspaceID string, limit, offset int) ([]domain.Campaign, int, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.ListCampaigns(authorized, workspaceID, limit, offset)
}

func (s *CampaignService) Start(ctx context.Context, workspaceID, campaignID string, version int) (*domain.CampaignRun, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	return s.snapshots.Start(authorized, workspaceID, campaignID, version)
}

func (s *CampaignService) GetRun(ctx context.Context, workspaceID, runID string) (*domain.CampaignRun, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.GetCampaignRun(authorized, workspaceID, runID)
}

func (s *CampaignService) Recipients(ctx context.Context, workspaceID, runID string, after int64, limit int) ([]domain.CampaignRecipientSnapshot, int64, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.ListCampaignSnapshots(authorized, workspaceID, runID, after, limit)
}

// PrepareBroadcast creates the compatibility Campaign Version and durable
// snapshot task used by the legacy Broadcast UI. The send task may be created
// immediately afterwards, but its orchestrator waits for this run to become
// dispatching before it reads any recipient.
func (s *CampaignService) PrepareBroadcast(ctx context.Context, workspaceID string, broadcast *domain.Broadcast) (*domain.CampaignRun, error) {
	if broadcast == nil {
		return nil, errors.New("broadcast is required")
	}
	listID := strings.TrimSpace(broadcast.Audience.List)
	audienceID := strings.TrimSpace(broadcast.Audience.AudienceID)
	hasList := listID != ""
	hasAudience := audienceID != "" && broadcast.Audience.AudienceVersion > 0
	if (audienceID != "" && !hasAudience) || hasList == hasAudience {
		return nil, errors.New("broadcast requires exactly one recipient source: a list or a versioned audience")
	}
	repository, ok := s.repository.(broadcastCampaignRepository)
	if !ok {
		return nil, errors.New("campaign repository does not support broadcast facade")
	}
	if s.tasks == nil {
		return nil, errors.New("campaign snapshot scheduler is unavailable")
	}
	if _, ok := s.repository.(broadcastCurrentCampaignRepository); ok {
		return s.prepareBroadcastCurrent(ctx, workspaceID, broadcast, broadcast.Audience.AudienceVersion, "")
	}
	variants := broadcastCampaignVariants(broadcast)
	version, err := repository.EnsureBroadcastCampaign(ctx, workspaceID, broadcast.ID, broadcast.Name,
		audienceID, broadcast.Audience.AudienceVersion, listID, broadcast.EffectiveChannelType(), variants)
	if err != nil {
		return nil, err
	}
	run, err := s.snapshots.StartAsync(ctx, workspaceID, version.CampaignID, version.Version)
	if err != nil {
		return nil, err
	}
	task := &domain.Task{ID: run.ID, WorkspaceID: workspaceID, Type: domain.SnapshotCampaignTaskType,
		Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 20, RetryInterval: 10,
		State: &domain.TaskState{SnapshotCampaign: &domain.SnapshotCampaignState{RunID: run.ID, BroadcastID: broadcast.ID}}}
	if err := s.tasks.CreateTask(ctx, workspaceID, task); err != nil {
		return nil, fmt.Errorf("create campaign snapshot task: %w", err)
	}
	return run, nil
}

// PrepareBroadcastExecution resolves a dynamic Audience only when the
// send_broadcast task actually starts. The build and version returned here are
// persisted on CampaignRun and remain fixed for the whole run.
func (s *CampaignService) PrepareBroadcastExecution(ctx context.Context, workspaceID string, broadcast *domain.Broadcast) (*domain.CampaignRun, error) {
	if broadcast == nil {
		return nil, errors.New("broadcast is required")
	}
	listID := strings.TrimSpace(broadcast.Audience.List)
	audienceID := strings.TrimSpace(broadcast.Audience.AudienceID)
	if (listID == "") == (audienceID == "") {
		return nil, errors.New("broadcast requires exactly one recipient source: a list or an audience")
	}
	if s.tasks == nil {
		return nil, errors.New("campaign snapshot scheduler is unavailable")
	}
	repository, ok := s.repository.(broadcastCampaignRepository)
	if !ok {
		return nil, errors.New("campaign repository does not support broadcast facade")
	}

	resolvedVersion := 0
	resolvedBuildID := ""
	if audienceID != "" {
		if s.audiences == nil {
			return nil, errors.New("audience execution service is unavailable")
		}
		build, err := s.audiences.ResolveLatestAndBuildInternal(ctx, workspaceID, audienceID)
		if err != nil {
			return nil, fmt.Errorf("resolve latest audience: %w", err)
		}
		resolvedVersion = build.AudienceVersion
		resolvedBuildID = build.ID
	}

	if _, ok := s.repository.(broadcastCurrentCampaignRepository); ok {
		return s.prepareBroadcastCurrent(ctx, workspaceID, broadcast, resolvedVersion, resolvedBuildID)
	}
	version, err := repository.EnsureBroadcastCampaign(ctx, workspaceID, broadcast.ID, broadcast.Name,
		audienceID, resolvedVersion, listID, broadcast.EffectiveChannelType(), broadcastCampaignVariants(broadcast))
	if err != nil {
		return nil, err
	}
	run, err := s.snapshots.StartAsyncResolved(ctx, workspaceID, version.CampaignID, version.Version, resolvedBuildID)
	if err != nil {
		return nil, err
	}
	task := &domain.Task{ID: run.ID, WorkspaceID: workspaceID, Type: domain.SnapshotCampaignTaskType,
		Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 20, RetryInterval: 10,
		State: &domain.TaskState{SnapshotCampaign: &domain.SnapshotCampaignState{RunID: run.ID, BroadcastID: broadcast.ID}}}
	if err := s.tasks.CreateTask(ctx, workspaceID, task); err != nil {
		return nil, fmt.Errorf("create campaign snapshot task: %w", err)
	}
	return run, nil
}

func broadcastCampaignVariants(broadcast *domain.Broadcast) []domain.CampaignVariant {
	count := len(broadcast.TestSettings.Variations)
	if count == 0 {
		return []domain.CampaignVariant{{ID: "default", WeightBP: 10_000}}
	}
	base := 10_000 / count
	remainder := 10_000 - base*count
	result := make([]domain.CampaignVariant, 0, count)
	seen := map[string]int{}
	for index, item := range broadcast.TestSettings.Variations {
		id := strings.TrimSpace(item.TemplateID)
		if id == "" {
			id = fmt.Sprintf("variant-%d", index+1)
		}
		seen[id]++
		if seen[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, seen[id])
		}
		weight := base
		if index == count-1 {
			weight += remainder
		}
		result = append(result, domain.CampaignVariant{ID: id, WeightBP: weight})
	}
	return result
}

type currentCampaignCreator interface {
	CreateCurrentCampaign(context.Context, string, domain.Campaign, domain.CampaignConfiguration) error
}
type broadcastCurrentCampaignRepository interface {
	EnsureBroadcastCurrentCampaign(context.Context, string, *domain.Broadcast, domain.CampaignConfiguration) (string, error)
}
type currentCampaignSaver interface {
	SaveCurrentCampaignConfiguration(context.Context, string, string, string, domain.CampaignConfiguration) error
}

func (s *CampaignService) Update(ctx context.Context, ws, id, name string, config domain.CampaignConfiguration) error {
	ctx, err := s.authorize(ctx, ws, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("campaign name is required")
	}
	repo, ok := s.repository.(currentCampaignSaver)
	if !ok {
		return errors.New("current configuration storage is unavailable")
	}
	if err := s.snapshots.validateTracking(ctx, ws, config); err != nil {
		return err
	}
	return repo.SaveCurrentCampaignConfiguration(ctx, ws, id, name, config)
}
func (s *CampaignService) Configuration(ctx context.Context, ws, id string) (*domain.CampaignConfiguration, error) {
	ctx, err := s.authorize(ctx, ws, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repository.(currentCampaignReader)
	if !ok {
		return nil, errors.New("current configuration storage is unavailable")
	}
	return repo.GetCurrentCampaignConfiguration(ctx, ws, id)
}
func (s *CampaignService) prepareBroadcastCurrent(ctx context.Context, ws string, broadcast *domain.Broadcast, version int, buildID string) (*domain.CampaignRun, error) {
	settings, err := json.Marshal(broadcast)
	if err != nil {
		return nil, err
	}
	variants := broadcastCampaignVariants(broadcast)
	for i, item := range broadcast.TestSettings.Variations {
		if i < len(variants) {
			variants[i].TemplateID = item.TemplateID
		}
	}
	config := domain.CampaignConfiguration{AudienceID: broadcast.Audience.AudienceID, AudienceVersion: version, ListID: broadcast.Audience.List, Channel: broadcast.EffectiveChannelType(), Variants: variants, Settings: settings}
	id, err := s.repository.(broadcastCurrentCampaignRepository).EnsureBroadcastCurrentCampaign(ctx, ws, broadcast, config)
	if err != nil {
		return nil, err
	}
	run, err := s.snapshots.StartConfiguration(ctx, ws, id, config, buildID)
	if err != nil {
		return nil, err
	}
	task := &domain.Task{ID: run.ID, WorkspaceID: ws, Type: domain.SnapshotCampaignTaskType, Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 20, RetryInterval: 10, State: &domain.TaskState{SnapshotCampaign: &domain.SnapshotCampaignState{RunID: run.ID, BroadcastID: broadcast.ID}}}
	if err = s.tasks.CreateTask(ctx, ws, task); err != nil {
		return nil, fmt.Errorf("create campaign snapshot task: %w", err)
	}
	return run, nil
}

// BindBroadcastExecution supplies the frozen settings and templates to all
// subsequent compilation in the orchestrator. Operational status stays live.
func (s *CampaignService) BindBroadcastExecution(ctx context.Context, ws string, broadcast *domain.Broadcast) (context.Context, *domain.Broadcast, error) {
	if broadcast.Audience.CampaignRunID == "" || s.snapshots.executions == nil {
		return ctx, broadcast, nil
	}
	snapshot, err := s.snapshots.executions.GetInternal(ctx, ws, broadcast.Audience.CampaignRunID)
	if errors.Is(err, domain.ErrExecutionSnapshotNotFound) {
		run, runErr := s.repository.GetCampaignRun(ctx, ws, broadcast.Audience.CampaignRunID)
		if runErr == nil && run.CampaignVersion > 0 {
			return ctx, broadcast, nil
		}
	}
	if err != nil {
		return ctx, nil, err
	}
	var config domain.CampaignConfiguration
	if err = json.Unmarshal(snapshot.Configuration, &config); err != nil {
		return ctx, nil, err
	}
	if len(config.Settings) == 0 {
		return ctx, nil, errors.New("broadcast execution configuration is missing")
	}
	var frozen domain.Broadcast
	if err = json.Unmarshal(config.Settings, &frozen); err != nil {
		return ctx, nil, err
	}
	frozen.Status = broadcast.Status
	frozen.WinningTemplate = broadcast.WinningTemplate
	frozen.Audience.CampaignRunID = broadcast.Audience.CampaignRunID
	frozen.Audience.AudienceVersion = broadcast.Audience.AudienceVersion
	frozen.Audience.AudienceBuildID = broadcast.Audience.AudienceBuildID
	bound, err := domain.WithExecutionSnapshot(ctx, snapshot)
	if err == nil {
		bound, err = s.snapshots.executions.bindRuntimeProviders(bound, ws, snapshot)
	}
	bound = domain.WithExecutionMediaArchive(bound, s.snapshots.executions.ArchiveRenderedMedia)
	return bound, &frozen, err
}
