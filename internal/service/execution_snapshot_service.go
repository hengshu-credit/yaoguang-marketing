package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/safehttpclient"
	"io"
	"net/http"
)

type ExecutionSnapshotService struct {
	workspaces domain.WorkspaceRepository
	repository domain.ExecutionSnapshotRepository
	auth       domain.AuthService
	templates  domain.TemplateRepository
	client     *http.Client
}

func NewExecutionSnapshotService(repository domain.ExecutionSnapshotRepository, auth domain.AuthService) (*ExecutionSnapshotService, error) {
	if repository == nil || auth == nil {
		return nil, errors.New("execution repository and authentication service are required")
	}
	return &ExecutionSnapshotService{repository: repository, auth: auth, client: safehttpclient.New()}, nil
}
func (s *ExecutionSnapshotService) SetTemplateRepository(templates domain.TemplateRepository) {
	s.templates = templates
}
func (s *ExecutionSnapshotService) GetInternal(ctx context.Context, ws, id string) (*domain.ExecutionSnapshot, error) {
	return s.repository.Get(ctx, ws, id)
}
func (s *ExecutionSnapshotService) authorize(ctx context.Context, ws string) (context.Context, *domain.UserWorkspace, error) {
	if s.auth == nil {
		return ctx, nil, errors.New("execution authentication service is unavailable")
	}
	ctx, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, ws)
	return ctx, membership, err
}
func executionReadPermission(m *domain.UserWorkspace, kind string) error {
	resource := domain.PermissionResourceBroadcasts
	if kind != "campaign" {
		resource = domain.PermissionResourceAutomations
	}
	if m == nil || !m.HasPermission(resource, domain.PermissionTypeRead) {
		return domain.NewPermissionError(resource, domain.PermissionTypeRead, "Insufficient permissions")
	}
	return nil
}
func (s *ExecutionSnapshotService) Get(ctx context.Context, ws, id, after string) (*domain.ExecutionDetails, error) {
	ctx, m, err := s.authorize(ctx, ws)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.repository.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	if err = executionReadPermission(m, snapshot.Kind); err != nil {
		return nil, err
	}
	// Configurations can include subject filters. Personalized evidence and render
	// variables require the same contact-read permission as Customer 360.
	if !m.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Contact read access is required for execution details")
	}
	evidence, next, err := s.repository.ListEvidence(ctx, ws, snapshot.ExecutionID, after, 100)
	if err != nil {
		return nil, err
	}
	return &domain.ExecutionDetails{Snapshot: snapshot, Evidence: evidence, Next: next}, nil
}
func (s *ExecutionSnapshotService) List(ctx context.Context, ws, source string) ([]domain.ExecutionSnapshot, error) {
	ctx, m, err := s.authorize(ctx, ws)
	if err != nil {
		return nil, err
	}
	if m == nil || !m.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Contact read access is required")
	}
	items, err := s.repository.List(ctx, ws, source, 50)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err = executionReadPermission(m, item.Kind); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (s *ExecutionSnapshotService) Asset(ctx context.Context, ws, executionID, hash string) (*domain.ExecutionAsset, error) {
	detail, err := s.Get(ctx, ws, executionID, "")
	if err != nil {
		return nil, err
	}
	var materials struct {
		Assets []struct {
			Hash string `json:"hash"`
		} `json:"assets"`
	}
	if err = json.Unmarshal(detail.Snapshot.Materials, &materials); err != nil {
		return nil, domain.ErrExecutionSnapshotNotFound
	}
	for _, asset := range materials.Assets {
		if asset.Hash == hash {
			return s.repository.GetAsset(ctx, ws, hash)
		}
	}
	if reader, ok := s.repository.(interface {
		ExecutionReferencesAsset(context.Context, string, string, string) (bool, error)
	}); ok {
		found, err := reader.ExecutionReferencesAsset(ctx, ws, executionID, hash)
		if err != nil {
			return nil, err
		}
		if found {
			return s.repository.GetAsset(ctx, ws, hash)
		}
	}
	return nil, domain.ErrExecutionSnapshotNotFound
}
func (s *ExecutionSnapshotService) AppendInternal(ctx context.Context, ws string, e domain.ExecutionEvidence) error {
	return s.repository.AppendEvidence(ctx, ws, e)
}
func (s *ExecutionSnapshotService) SaveInternal(ctx context.Context, ws string, snapshot *domain.ExecutionSnapshot) error {
	return s.repository.Save(ctx, ws, snapshot)
}

type ExecutionTemplateReference struct {
	ID      string
	Version int64
}

func (s *ExecutionSnapshotService) Build(ctx context.Context, ws, kind, id, source string, config any, refs []ExecutionTemplateReference) (*domain.ExecutionSnapshot, error) {
	templates := []*domain.Template{}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := fmt.Sprintf("%s:%d", ref.ID, ref.Version)
		if seen[key] {
			continue
		}
		seen[key] = true
		if s.templates == nil {
			return nil, errors.New("template snapshot source is unavailable")
		}
		template, err := s.templates.GetTemplateByID(ctx, ws, ref.ID, ref.Version)
		if err != nil {
			return nil, fmt.Errorf("snapshot template %s: %w", ref.ID, err)
		}
		templates = append(templates, template)
	}
	snapshot, err := domain.NewExecutionSnapshot(kind, id, config, templates)
	if err != nil {
		return nil, err
	}
	snapshot.SourceID = source
	references, err := executionMediaReferences(snapshot.Materials, true)
	if err != nil {
		return nil, err
	}
	assets, err := s.archiveMediaSet(ctx, ws, references, nil)
	if err != nil {
		return nil, err
	}
	snapshot.Materials, err = domain.SanitizeExecutionJSON(map[string]any{"templates": templates, "assets": assets})
	if err != nil {
		return nil, err
	}
	if kind == "campaign" && len(templates) > 0 {
		if err = s.captureCampaignProviders(ctx, ws, snapshot); err != nil {
			return nil, err
		}
	}
	if err = snapshot.Rehash(); err != nil {
		return nil, err
	}
	return snapshot, nil
}
func (s *ExecutionSnapshotService) archive(ctx context.Context, ws, url string) (*domain.ExecutionAsset, error) {
	client := s.client
	if client == nil {
		client = safehttpclient.New()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media returned HTTP %d", response.StatusCode)
	}
	const maxAsset = 20 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAsset+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAsset {
		return nil, errors.New("execution media exceeds 20 MiB")
	}
	sum := sha256.Sum256(data)
	asset := domain.ExecutionAsset{Hash: hex.EncodeToString(sum[:]), MediaType: response.Header.Get("Content-Type"), Data: data}
	if asset.MediaType == "" {
		asset.MediaType = http.DetectContentType(data)
	}
	if err = s.repository.SaveAsset(ctx, ws, asset); err != nil {
		return nil, err
	}
	return &asset, nil
}
