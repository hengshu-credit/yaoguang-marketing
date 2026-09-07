package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type executionTemplateArchiveWriter interface {
	SaveTemplateMaterialArchive(context.Context, string, *domain.Template, json.RawMessage) error
}

func (s *ExecutionSnapshotService) PrepareTemplate(ctx context.Context, ws string, template *domain.Template) error {
	writer, ok := s.repository.(executionTemplateArchiveWriter)
	if !ok {
		return errors.New("template material archive storage is unavailable")
	}
	body, err := domain.SanitizeExecutionJSON(template)
	if err != nil {
		return err
	}
	references, err := executionMediaReferences(body, true)
	if err != nil {
		return err
	}
	assets, err := s.archiveMediaSet(ctx, ws, references, nil)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(assets)
	if err != nil {
		return err
	}
	return writer.SaveTemplateMaterialArchive(ctx, ws, template, encoded)
}
func (s *ExecutionSnapshotService) PrepareAutomation(ctx context.Context, ws string, automation *domain.Automation) error {
	if s.templates == nil {
		return errors.New("template archive source unavailable")
	}
	if err := s.archiveRuntimeNodes(ctx, ws, automation); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, node := range automation.Nodes {
		id, _ := node.Config["template_id"].(string)
		if id == "" {
			continue
		}
		version, _ := audienceContextInt(node.Config["template_version"])
		key := fmt.Sprintf("%s:%d", id, version)
		if seen[key] {
			continue
		}
		seen[key] = true
		template, err := s.templates.GetTemplateByID(ctx, ws, id, int64(version))
		if err != nil {
			return err
		}
		if err = s.PrepareTemplate(ctx, ws, template); err != nil {
			return err
		}
	}
	return nil
}
func (s *TemplateService) SetExecutionSnapshotService(snapshots *ExecutionSnapshotService) {
	s.executionSnapshots = snapshots
}
func WithAutomationMaterialArchives(snapshots *ExecutionSnapshotService) AutomationServiceOption {
	return func(s *AutomationService) { s.executionSnapshots = snapshots }
}
