package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

const (
	defaultOutboxMaxAttempts = 20
	defaultOutboxPoll        = 100 * time.Millisecond
	maxOutboxErrorLength     = 2_000
	outboxWorkspaceCursor    = "outbox-relay"
)

type OutboxRelay struct {
	workspaces domain.WorkspaceCursorRepository
	repository domain.RealtimeRepository
	publisher  broker.Publisher
	workerID   string
	batchSize  int
	lease      time.Duration

	maxAttempts  int
	pollInterval time.Duration
	now          func() time.Time
	retryBackoff func(int, uuid.UUID) time.Duration
	processMu    sync.Mutex
}

func NewOutboxRelay(
	workspaces domain.WorkspaceCursorRepository,
	repository domain.RealtimeRepository,
	publisher broker.Publisher,
	workerID string,
	batchSize int,
	lease time.Duration,
) (*OutboxRelay, error) {
	if workspaces == nil {
		return nil, errors.New("outbox relay workspace cursor is required")
	}
	if repository == nil {
		return nil, errors.New("outbox relay repository is required")
	}
	if publisher == nil {
		return nil, errors.New("outbox relay publisher is required")
	}
	if workerID == "" {
		return nil, errors.New("outbox relay worker id is required")
	}
	if batchSize <= 0 {
		return nil, errors.New("outbox relay batch size must be positive")
	}
	if lease <= 0 {
		return nil, errors.New("outbox relay lease must be positive")
	}
	return &OutboxRelay{
		workspaces:   workspaces,
		repository:   repository,
		publisher:    publisher,
		workerID:     workerID,
		batchSize:    batchSize,
		lease:        lease,
		maxAttempts:  defaultOutboxMaxAttempts,
		pollInterval: defaultOutboxPoll,
		now:          func() time.Time { return time.Now().UTC() },
		retryBackoff: outboxRetryBackoff,
	}, nil
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	for {
		processed, err := r.ProcessOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if processed > 0 && err == nil {
			continue
		}

		timer := time.NewTimer(r.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (r *OutboxRelay) ProcessOnce(ctx context.Context) (int, error) {
	// The cursor is persisted by WorkspaceCursorRepository. Serializing this relay
	// instance also prevents overlapping scheduler ticks from consuming its batch.
	r.processMu.Lock()
	defer r.processMu.Unlock()

	workspaceIDs, err := r.workspaces.NextWorkspaceIDs(ctx, outboxWorkspaceCursor, r.batchSize)
	if err != nil {
		return 0, fmt.Errorf("advance outbox workspace cursor: %w", err)
	}
	workspaceIDs = normalizedWorkspaceIDs(workspaceIDs)
	if len(workspaceIDs) == 0 {
		return 0, nil
	}

	fairShare := (r.batchSize + len(workspaceIDs) - 1) / len(workspaceIDs)
	remaining := r.batchSize
	processed := 0
	var processErrors []error
	now := r.now().UTC()

	for offset := 0; offset < len(workspaceIDs) && remaining > 0; offset++ {
		workspaceID := workspaceIDs[offset]
		claimLimit := min(fairShare, remaining)
		messages, claimErr := r.repository.ClaimOutbox(
			ctx, workspaceID, r.workerID, now, r.lease, claimLimit,
		)
		if claimErr != nil {
			processErrors = append(processErrors, fmt.Errorf("claim outbox for workspace %s: %w", workspaceID, claimErr))
			continue
		}

		for _, message := range messages {
			processed++
			remaining--
			if publishErr := r.processMessage(ctx, workspaceID, message, now); publishErr != nil {
				processErrors = append(processErrors, publishErr)
			}
			if remaining == 0 {
				break
			}
		}
	}

	return processed, errors.Join(processErrors...)
}

func (r *OutboxRelay) processMessage(
	ctx context.Context,
	workspaceID string,
	message domain.OutboxMessage,
	now time.Time,
) error {
	if message.ClaimToken == nil || *message.ClaimToken == uuid.Nil {
		return fmt.Errorf("outbox message %s has no claim token", message.ID)
	}

	publishedMessage, err := outboxBrokerMessage(workspaceID, message)
	if err != nil {
		return r.releaseFailed(ctx, workspaceID, message, now, err)
	}
	if err := r.publisher.Publish(ctx, publishedMessage); err != nil {
		return r.releaseFailed(ctx, workspaceID, message, now, err)
	}

	marked, err := r.repository.MarkOutboxPublished(
		ctx, workspaceID, message.ID, *message.ClaimToken, now,
	)
	if err != nil {
		return fmt.Errorf("mark outbox message %s published: %w", message.ID, err)
	}
	if !marked {
		return fmt.Errorf("outbox claim lost after publishing message %s", message.ID)
	}
	return nil
}

func (r *OutboxRelay) releaseFailed(
	ctx context.Context,
	workspaceID string,
	message domain.OutboxMessage,
	now time.Time,
	cause error,
) error {
	dead := message.Attempts >= r.maxAttempts
	availableAt := now
	if !dead {
		availableAt = now.Add(r.retryBackoff(message.Attempts, message.ID))
	}
	lastError := truncateOutboxError(cause.Error())
	released, releaseErr := r.repository.ReleaseOutbox(
		ctx, workspaceID, message.ID, *message.ClaimToken, availableAt, lastError, dead,
	)
	if releaseErr != nil {
		return errors.Join(
			fmt.Errorf("process outbox message %s: %w", message.ID, cause),
			fmt.Errorf("release outbox message %s: %w", message.ID, releaseErr),
		)
	}
	if !released {
		return errors.Join(
			fmt.Errorf("process outbox message %s: %w", message.ID, cause),
			fmt.Errorf("outbox claim lost while releasing message %s", message.ID),
		)
	}
	return fmt.Errorf("process outbox message %s: %w", message.ID, cause)
}

func outboxBrokerMessage(workspaceID string, message domain.OutboxMessage) (broker.Message, error) {
	if message.ID == uuid.Nil {
		return broker.Message{}, errors.New("outbox message id is required")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(message.Payload, &document); err != nil {
		return broker.Message{}, fmt.Errorf("decode outbox payload: %w", err)
	}
	if document == nil {
		return broker.Message{}, errors.New("outbox payload must be a JSON object")
	}
	workspaceJSON, _ := json.Marshal(workspaceID)
	document["workspace_id"] = workspaceJSON
	body, err := json.Marshal(document)
	if err != nil {
		return broker.Message{}, fmt.Errorf("encode workspace outbox payload: %w", err)
	}

	var metadata struct {
		Type          string    `json:"type"`
		SchemaVersion int       `json:"schema_version"`
		CorrelationID uuid.UUID `json:"correlation_id"`
		OccurredAt    time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return broker.Message{}, fmt.Errorf("decode outbox metadata: %w", err)
	}

	headers := make(map[string]any)
	if len(message.Headers) > 0 {
		decoderErr := json.Unmarshal(message.Headers, &headers)
		if decoderErr != nil {
			return broker.Message{}, fmt.Errorf("decode outbox headers: %w", decoderErr)
		}
	}
	if headers == nil {
		headers = make(map[string]any)
	}
	headers["workspace_id"] = workspaceID
	headers["outbox_attempt"] = message.Attempts

	return broker.Message{
		ID:            message.ID,
		CorrelationID: metadata.CorrelationID,
		Exchange:      message.Topic,
		RoutingKey:    message.RoutingKey,
		Type:          metadata.Type,
		SchemaVersion: metadata.SchemaVersion,
		Timestamp:     metadata.OccurredAt,
		Headers:       headers,
		Body:          body,
	}, nil
}

func normalizedWorkspaceIDs(workspaceIDs []string) []string {
	seen := make(map[string]struct{}, len(workspaceIDs))
	ids := make([]string, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		if workspaceID == "" {
			continue
		}
		if _, exists := seen[workspaceID]; exists {
			continue
		}
		seen[workspaceID] = struct{}{}
		ids = append(ids, workspaceID)
	}
	return ids
}

func outboxRetryBackoff(attempt int, messageID uuid.UUID) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 11)
	base := time.Second * time.Duration(1<<shift)
	if base > 30*time.Minute {
		base = 30 * time.Minute
	}
	// Stable per-message jitter prevents a recovered broker from receiving every
	// failed row at once while keeping retry schedules reproducible.
	jitterPercent := 80 + int(messageID[0])%41
	backoff := time.Duration(int64(base) * int64(jitterPercent) / 100)
	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}
	return backoff
}

func truncateOutboxError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= maxOutboxErrorLength {
		return message
	}
	return message[:maxOutboxErrorLength]
}
