package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

type StateExpiryProjector interface {
	Tick(context.Context, string, time.Time) error
}

// SubjectStateMaintenanceWorker consumes persisted next-check times, so expiry
// and attribute-only updates do not require another event to arrive.
type SubjectStateMaintenanceWorker struct {
	workspaces domain.WorkspaceCursorRepository
	states     StateExpiryProjector
	logger     logger.Logger
	interval   time.Duration
	now        func() time.Time
}

func NewSubjectStateMaintenanceWorker(workspaces domain.WorkspaceCursorRepository, states StateExpiryProjector, log logger.Logger) *SubjectStateMaintenanceWorker {
	return &SubjectStateMaintenanceWorker{workspaces: workspaces, states: states, logger: log, interval: 5 * time.Second, now: time.Now}
}

func (w *SubjectStateMaintenanceWorker) ProcessOnce(ctx context.Context) (int, error) {
	ids, err := w.workspaces.NextWorkspaceIDs(ctx, "subject-state-maintenance", 50)
	if err != nil {
		return 0, err
	}
	var failures []error
	completed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return completed, ctx.Err()
		}
		if err := w.states.Tick(ctx, id, w.now().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("workspace %s: %w", id, err))
		} else {
			completed++
		}
	}
	return completed, errors.Join(failures...)
}

func (w *SubjectStateMaintenanceWorker) Start(ctx context.Context) {
	timer := time.NewTicker(w.interval)
	defer timer.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil && w.logger != nil {
			w.logger.WithField("error", err.Error()).Error("State maintenance failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
