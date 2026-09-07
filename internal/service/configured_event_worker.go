package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"time"
)

type ConfiguredEventHandler interface {
	Handle(context.Context, broker.Message) error
}

// ConfiguredEventWorker consumes a transactional fan-out of the existing outbox.
// It runs in both legacy and realtime installations; collection and analytics
// keep their own delivery acknowledgement independently.
type ConfiguredEventWorker struct {
	workspaces domain.WorkspaceCursorRepository
	queue      domain.ConfiguredEventQueue
	handler    ConfiguredEventHandler
	logger     logger.Logger
}

func NewConfiguredEventWorker(ws domain.WorkspaceCursorRepository, q domain.ConfiguredEventQueue, h ConfiguredEventHandler, l logger.Logger) *ConfiguredEventWorker {
	return &ConfiguredEventWorker{ws, q, h, l}
}

func (w *ConfiguredEventWorker) ProcessOnce(ctx context.Context) (int, error) {
	ids, err := w.workspaces.NextWorkspaceIDs(ctx, "configured-event-worker", 50)
	if err != nil {
		return 0, err
	}
	completed := 0
	var failures []error
	for _, ws := range ids {
		jobs, err := w.queue.ClaimConfiguredEvents(ctx, ws, time.Now().UTC(), time.Minute, 100)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return completed, ctx.Err()
			}
			var envelope domain.EventEnvelope
			err := json.Unmarshal(job.Payload, &envelope)
			if err == nil {
				// SQL timeline emitters use current_database(); the assigned workspace
				// cursor is the authority for the tenant connection.
				envelope.WorkspaceID = ws
				payload, marshalErr := json.Marshal(envelope)
				if marshalErr != nil {
					err = marshalErr
				} else {
					err = w.handler.Handle(ctx, broker.Message{ID: job.ID, Body: payload})
				}
			}
			at := time.Now().UTC()
			reason := ""
			if err != nil {
				reason = err.Error()
				at = at.Add(30 * time.Second)
				failures = append(failures, fmt.Errorf("workspace %s event %s: %w", ws, job.ID, err))
			}
			if finishErr := w.queue.FinishConfiguredEvent(ctx, ws, job, at, reason); finishErr != nil {
				failures = append(failures, finishErr)
			} else if err == nil {
				completed++
			}
		}
	}
	return completed, errors.Join(failures...)
}

func (w *ConfiguredEventWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil && w.logger != nil {
			w.logger.WithField("error", err.Error()).Error("Configured event processing failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
