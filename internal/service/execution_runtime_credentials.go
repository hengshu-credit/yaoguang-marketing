package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
)

type executionRuntimeStorage interface {
	SaveRuntimeNodes(context.Context, string, string, json.RawMessage, string, bool) error
	RuntimeNodes(context.Context, string, string) (string, error)
}

func (s *ExecutionSnapshotService) SetWorkspaceRepository(w domain.WorkspaceRepository) {
	s.workspaces = w
}
func (s *ExecutionSnapshotService) archiveRuntimeNodes(ctx context.Context, ws string, a *domain.Automation) error {
	configs := map[string]map[string]interface{}{}
	needsProviders := false
	for _, node := range a.Nodes {
		configs[node.ID] = node.Config
		if id, _ := node.Config["integration_id"].(string); id != "" || node.Type == domain.NodeTypeEmail {
			needsProviders = true
		}
	}
	raw, err := json.Marshal(configs)
	if err != nil {
		return err
	}
	redacted, err := domain.SanitizeExecutionJSON(configs)
	if err != nil {
		return err
	}
	if bytes.Equal(raw, redacted) && !needsProviders {
		return nil
	}
	if s.workspaces == nil {
		return errors.New("runtime credential key source unavailable")
	}
	storage, ok := s.repository.(executionRuntimeStorage)
	if !ok {
		return errors.New("runtime credential archive unavailable")
	}
	workspace, err := s.workspaces.GetByID(ctx, ws)
	if err != nil {
		return err
	}
	archive := map[string]any{}
	for id, config := range configs {
		archive[id] = config
	}
	if needsProviders {
		providers := domain.ExecutionProviders{Integrations: domain.Integrations{}, MarketingEmailProviderID: workspace.Settings.MarketingEmailProviderID}
		seen := map[string]bool{}
		for _, node := range a.Nodes {
			id, _ := node.Config["integration_id"].(string)
			if id == "" && node.Type == domain.NodeTypeEmail {
				id = workspace.Settings.MarketingEmailProviderID
			}
			if id == "" || seen[id] {
				continue
			}
			integration := workspace.GetIntegrationByID(id)
			if integration == nil {
				// A deleted integration is frozen as unavailable, never replaced with live credentials.
				continue
			}
			providers.Integrations = append(providers.Integrations, *integration)
			seen[id] = true
		}
		archive["__execution_providers"] = providers
	}
	archiveJSON, err := json.Marshal(archive)
	if err != nil {
		return err
	}
	encrypted, err := pkgcrypto.EncryptString(string(archiveJSON), workspace.Settings.SecretKey)
	if err != nil {
		return err
	}
	return storage.SaveRuntimeNodes(ctx, ws, a.ID, raw, encrypted, needsProviders)
}
func (s *ExecutionSnapshotService) hydrateRuntimeNodes(ctx context.Context, ws string, snapshot *domain.ExecutionSnapshot, a *domain.Automation) error {
	var deps struct {
		Ref string `json:"runtime_nodes_ref"`
	}
	if err := json.Unmarshal(snapshot.Dependencies, &deps); err != nil {
		return err
	}
	if deps.Ref == "" {
		return nil
	}
	storage, ok := s.repository.(executionRuntimeStorage)
	if !ok || s.workspaces == nil {
		return errors.New("runtime credential archive unavailable")
	}
	encrypted, err := storage.RuntimeNodes(ctx, ws, deps.Ref)
	if err != nil {
		return err
	}
	workspace, err := s.workspaces.GetByID(ctx, ws)
	if err != nil {
		return err
	}
	raw, err := pkgcrypto.DecryptFromHexString(encrypted, workspace.Settings.SecretKey)
	if err != nil {
		return fmt.Errorf("decrypt execution credentials: %w", err)
	}
	var configs map[string]json.RawMessage
	if err = json.Unmarshal([]byte(raw), &configs); err != nil {
		return err
	}
	for _, node := range a.Nodes {
		if encoded, ok := configs[node.ID]; ok {
			var config map[string]interface{}
			if err = json.Unmarshal(encoded, &config); err != nil {
				return err
			}
			redacted, err := domain.SanitizeExecutionJSON(config)
			if err != nil {
				return err
			}
			expected, err := domain.SanitizeExecutionJSON(node.Config)
			if err != nil {
				return err
			}
			if !bytes.Equal(redacted, expected) {
				return errors.New("runtime credentials do not match immutable configuration")
			}
			node.Config = config
		}
	}
	return nil
}

func (s *ExecutionSnapshotService) bindRuntimeProviders(ctx context.Context, ws string, snapshot *domain.ExecutionSnapshot) (context.Context, error) {
	var deps struct {
		Ref string `json:"runtime_nodes_ref"`
	}
	if err := json.Unmarshal(snapshot.Dependencies, &deps); err != nil {
		return ctx, err
	}
	if deps.Ref == "" {
		return ctx, nil
	}
	storage, ok := s.repository.(executionRuntimeStorage)
	if !ok || s.workspaces == nil {
		return ctx, errors.New("runtime credential archive unavailable")
	}
	encrypted, err := storage.RuntimeNodes(ctx, ws, deps.Ref)
	if err != nil {
		return ctx, err
	}
	workspace, err := s.workspaces.GetByID(ctx, ws)
	if err != nil {
		return ctx, err
	}
	raw, err := pkgcrypto.DecryptFromHexString(encrypted, workspace.Settings.SecretKey)
	if err != nil {
		return ctx, err
	}
	var archive struct {
		Providers *domain.ExecutionProviders `json:"__execution_providers"`
	}
	if err = json.Unmarshal([]byte(raw), &archive); err != nil {
		return ctx, err
	}
	if archive.Providers == nil {
		return ctx, nil
	}
	return domain.WithExecutionProviders(ctx, *archive.Providers), nil
}

func (s *ExecutionSnapshotService) captureCampaignProviders(ctx context.Context, ws string, snapshot *domain.ExecutionSnapshot) error {
	reader, ok := s.repository.(interface {
		LatestRuntimeReference(context.Context, string, string) (string, error)
	})
	if !ok {
		return errors.New("execution provider archive unavailable")
	}
	config := &domain.Automation{ID: snapshot.ExecutionID, Nodes: []*domain.AutomationNode{{ID: "campaign-email", Type: domain.NodeTypeEmail, Config: map[string]interface{}{}}}}
	if err := s.archiveRuntimeNodes(ctx, ws, config); err != nil {
		return err
	}
	reference, err := reader.LatestRuntimeReference(ctx, ws, snapshot.ExecutionID)
	if err != nil {
		return err
	}
	var dependencies map[string]any
	if err = json.Unmarshal(snapshot.Dependencies, &dependencies); err != nil {
		return err
	}
	if dependencies == nil {
		dependencies = map[string]any{}
	}
	dependencies["runtime_nodes_ref"] = reference
	snapshot.Dependencies, err = json.Marshal(dependencies)
	return err
}
func (s *ExecutionSnapshotService) ResolveExecutionIntegration(ctx context.Context, ws, executionID, integrationID string, current *domain.Workspace) (*domain.Integration, error) {
	snapshot, err := s.GetInternal(ctx, ws, executionID)
	if err != nil {
		return nil, err
	}
	bound, err := s.bindRuntimeProviders(ctx, ws, snapshot)
	if err != nil {
		return nil, err
	}
	frozen := domain.ExecutionProviderWorkspace(bound, current)
	if frozen == current {
		return nil, errors.New("execution provider archive is missing")
	}
	integration := frozen.GetIntegrationByID(integrationID)
	if integration == nil {
		return nil, errors.New("execution provider integration is unavailable")
	}
	return integration, nil
}
