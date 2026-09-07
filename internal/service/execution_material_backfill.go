package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"time"
)

type AutomationArchiveBacklog interface {
	UnarchivedAutomationTemplates(context.Context, string, int) ([]*domain.Template, error)
}
type ExecutionMaterialBackfillWorker struct {
	workspaces domain.WorkspaceCursorRepository
	backlog    AutomationArchiveBacklog
	snapshots  *ExecutionSnapshotService
	log        logger.Logger
}

func NewExecutionMaterialBackfillWorker(w domain.WorkspaceCursorRepository, b AutomationArchiveBacklog, s *ExecutionSnapshotService, l logger.Logger) *ExecutionMaterialBackfillWorker {
	return &ExecutionMaterialBackfillWorker{w, b, s, l}
}
func (w *ExecutionMaterialBackfillWorker) ProcessOnce(ctx context.Context) (int, error) {
	ids, err := w.workspaces.NextWorkspaceIDs(ctx, "execution-material-backfill", 10)
	if err != nil {
		return 0, err
	}
	completed := 0
	var failures []error
	for _, id := range ids {
		if ctx.Err() != nil {
			return completed, ctx.Err()
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		if refreshErr := w.snapshots.RetryProviderRefresh(bounded, id); refreshErr != nil {
			failures = append(failures, refreshErr)
		}
		templates, err := w.backlog.UnarchivedAutomationTemplates(bounded, id, 25)
		if err == nil {
			for _, template := range templates {
				if err = w.snapshots.PrepareTemplate(bounded, id, template); err != nil {
					break
				}
				completed++
			}
		}
		if backlog, ok := w.backlog.(interface {
			UnarchivedRuntimeAutomations(context.Context, string, int) ([]*domain.Automation, error)
		}); ok {
			automations, loadErr := backlog.UnarchivedRuntimeAutomations(bounded, id, 25)
			if loadErr != nil {
				failures = append(failures, loadErr)
			} else {
				for _, automation := range automations {
					if archiveErr := w.snapshots.archiveRuntimeNodes(bounded, id, automation); archiveErr != nil {
						failures = append(failures, archiveErr)
					} else {
						completed++
					}
				}
			}
		}
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("archive workspace %s: %w", id, err))
		}
	}
	return completed, errors.Join(failures...)
}
func (w *ExecutionMaterialBackfillWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil && w.log != nil {
			w.log.WithField("error", err.Error()).Error("Execution material backfill failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
