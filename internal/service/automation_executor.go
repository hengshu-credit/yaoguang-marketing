package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// AutomationExecutor processes contacts through automation workflows
type AutomationExecutor struct {
	automationRepo  domain.AutomationRepository
	contactRepo     domain.ContactRepository
	workspaceRepo   domain.WorkspaceRepository
	contactListRepo domain.ContactListRepository
	templateRepo    domain.TemplateRepository
	emailQueueRepo  domain.EmailQueueRepository
	timelineRepo    domain.ContactTimelineRepository
	nodeExecutors   map[domain.NodeType]NodeExecutor
	logger          logger.Logger
	apiEndpoint     string
}

// NewAutomationExecutor creates a new AutomationExecutor
func NewAutomationExecutor(
	automationRepo domain.AutomationRepository,
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	contactListRepo domain.ContactListRepository,
	listRepo domain.ListRepository,
	templateRepo domain.TemplateRepository,
	emailQueueRepo domain.EmailQueueRepository,
	timelineRepo domain.ContactTimelineRepository,
	log logger.Logger,
	apiEndpoint string,
) *AutomationExecutor {
	qb := NewQueryBuilder()

	executors := map[domain.NodeType]NodeExecutor{
		domain.NodeTypeTrigger:          NewTriggerNodeExecutor(),
		domain.NodeTypeDelay:            NewDelayNodeExecutor(),
		domain.NodeTypeEmail:            NewEmailNodeExecutor(emailQueueRepo, templateRepo, workspaceRepo, listRepo, contactListRepo, apiEndpoint, log),
		domain.NodeTypeBranch:           NewBranchNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeFilter:           NewFilterNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeAddToList:        NewAddToListNodeExecutor(contactListRepo),
		domain.NodeTypeRemoveFromList:   NewRemoveFromListNodeExecutor(contactListRepo),
		domain.NodeTypeABTest:           NewABTestNodeExecutor(),
		domain.NodeTypeWebhook:          NewWebhookNodeExecutor(log),
		domain.NodeTypeListStatusBranch: NewListStatusBranchNodeExecutor(contactListRepo),
	}

	return &AutomationExecutor{
		automationRepo:  automationRepo,
		contactRepo:     contactRepo,
		workspaceRepo:   workspaceRepo,
		contactListRepo: contactListRepo,
		templateRepo:    templateRepo,
		emailQueueRepo:  emailQueueRepo,
		timelineRepo:    timelineRepo,
		nodeExecutors:   executors,
		logger:          log,
		apiEndpoint:     apiEndpoint,
	}
}

// Execute processes a contact through their automation nodes until a delay or completion.
// It loops through multiple nodes in a single tick for efficiency, persisting state after each node.
func (e *AutomationExecutor) Execute(ctx context.Context, workspaceID string, contactAutomation *domain.ContactAutomation) error {
	// Get automation once (outside loop)
	automation, err := e.automationRepo.GetByID(ctx, workspaceID, contactAutomation.AutomationID)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get automation")
	}

	// Check if automation is paused/not live
	// When paused, contacts stay frozen at their current node (they don't get exited)
	if automation.Status != domain.AutomationStatusLive {
		return nil
	}

	// Early exit if already completed (no current node) - avoid fetching contact unnecessarily
	if contactAutomation.CurrentNodeID == nil {
		return e.markAsCompleted(ctx, workspaceID, contactAutomation, "completed")
	}

	// Get contact data once (outside loop) - only if we have nodes to process
	contactData, err := e.contactRepo.GetContactByEmail(ctx, workspaceID, contactAutomation.ContactEmail)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get contact")
	}

	// LOOP: Process nodes until delay, completion, or max iterations
	const maxNodesPerTick = 10
	for iterations := 0; iterations < maxNodesPerTick; iterations++ {

		// Get current node from embedded nodes
		node := automation.GetNodeByID(*contactAutomation.CurrentNodeID)
		if node == nil {
			return e.markAsExited(ctx, workspaceID, contactAutomation, "automation_node_deleted")
		}

		// Get executor for node type
		executor, ok := e.nodeExecutors[node.Type]
		if !ok {
			return e.handleError(ctx, workspaceID, contactAutomation,
				fmt.Errorf("unsupported node type: %s", node.Type), "unsupported node type")
		}

		// Create node execution entry (processing)
		nodeExecution := e.createNodeExecution(contactAutomation, node, domain.NodeActionProcessing)
		nodeStartTime := time.Now()
		_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, nodeExecution)

		// Build context from previous node executions
		executionContext, err := e.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomation.ID)
		if err != nil {
			e.logger.WithField("error", err).Warn("Failed to build context from node executions")
			executionContext = make(map[string]interface{})
		}

		// Execute the node
		params := NodeExecutionParams{
			WorkspaceID:      workspaceID,
			Contact:          contactAutomation,
			Node:             node,
			Automation:       automation,
			ContactData:      contactData,
			ExecutionContext: executionContext,
		}
		result, execErr := executor.Execute(ctx, params)

		// Handle execution error
		if execErr != nil {
			nodeExecution.Action = domain.NodeActionFailed
			nodeExecution.Error = strPtr(execErr.Error())
			completedAt := time.Now().UTC()
			nodeExecution.CompletedAt = &completedAt
			_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)
			return e.handleError(ctx, workspaceID, contactAutomation, execErr, "node execution failed")
		}

		// Update contact automation state
		contactAutomation.CurrentNodeID = result.NextNodeID
		contactAutomation.ScheduledAt = result.ScheduledAt
		if result.ExitReason != nil {
			contactAutomation.ExitReason = result.ExitReason
		}

		// Determine status (terminal node = completed, unless waiting for a delay)
		isTerminalNode := result.NextNodeID == nil && result.Status == domain.ContactAutomationStatusActive
		isWaitingDelay := result.ScheduledAt != nil && result.ScheduledAt.After(time.Now())
		if isTerminalNode && !isWaitingDelay {
			contactAutomation.Status = domain.ContactAutomationStatusCompleted
		} else {
			contactAutomation.Status = result.Status
		}

		// PERSIST STATE (critical for crash recovery). Optimistic lock on
		// status='active' so a concurrent exit (e.g. a stop-on-reply interrupt that
		// landed mid-tick) is not clobbered back to active/next-node.
		updated, err := e.automationRepo.UpdateContactAutomationIfActive(ctx, workspaceID, contactAutomation)
		if err != nil {
			return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to update contact automation")
		}
		if !updated {
			// The journey was exited concurrently; stop processing this tick.
			e.logger.WithField("contact_automation_id", contactAutomation.ID).
				Info("Contact automation no longer active (exited concurrently); aborting tick")
			return nil
		}

		// Update node execution to completed
		duration := time.Since(nodeStartTime).Milliseconds()
		nodeExecution.Action = domain.NodeActionCompleted
		completedAt := time.Now().UTC()
		nodeExecution.CompletedAt = &completedAt
		nodeExecution.DurationMs = &duration
		nodeExecution.Output = result.Output
		_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)

		// EXIT: Completed (terminal node reached)
		if contactAutomation.Status == domain.ContactAutomationStatusCompleted {
			_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "completed")
			e.createAutomationEndEvent(ctx, workspaceID, contactAutomation, "completed")
			return nil
		}

		// EXIT: Exited (filter/branch exit)
		if contactAutomation.Status == domain.ContactAutomationStatusExited {
			_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "exited")
			reason := "exited"
			if contactAutomation.ExitReason != nil {
				reason = *contactAutomation.ExitReason
			}
			e.createAutomationEndEvent(ctx, workspaceID, contactAutomation, reason)
			return nil
		}

		// EXIT: Delay node (ScheduledAt is in the future)
		if result.ScheduledAt != nil && result.ScheduledAt.After(time.Now()) {
			return nil
		}

		// CONTINUE: Process next node immediately
	}

	// Hit max iterations - remaining nodes picked up next tick
	// State already persisted, so this is safe
	return nil
}

// PlanClaimedJourneyStep evaluates one PostgreSQL-authorized state transition.
// It never performs an external action directly: email, webhook, and list
// mutations become deterministic side-effect/outbox records which commit with
// the new journey state.
func (e *AutomationExecutor) PlanClaimedJourneyStep(
	ctx context.Context,
	workspaceID string,
	claim domain.ContactAutomationClaim,
	now time.Time,
) (domain.JourneyStateCommit, error) {
	now = now.UTC()
	next := claim.ContactAutomation
	commit := domain.JourneyStateCommit{ContactAutomation: next}

	if next.Status != domain.ContactAutomationStatusActive {
		return commit, fmt.Errorf("contact automation %s is not active", next.ID)
	}
	if next.CurrentNodeID == nil {
		commit.ContactAutomation.Status = domain.ContactAutomationStatusCompleted
		commit.ContactAutomation.ScheduledAt = nil
		return commit, nil
	}

	automation, err := e.automationRepo.GetByID(ctx, workspaceID, next.AutomationID)
	if err != nil {
		return commit, fmt.Errorf("load claimed automation: %w", err)
	}
	if automation.Status != domain.AutomationStatusLive {
		return commit, fmt.Errorf("automation %s is not live", automation.ID)
	}
	node := automation.GetNodeByID(*next.CurrentNodeID)
	if node == nil {
		reason := "automation_node_deleted"
		commit.ContactAutomation.Status = domain.ContactAutomationStatusExited
		commit.ContactAutomation.ExitReason = &reason
		commit.ContactAutomation.ScheduledAt = nil
		return commit, nil
	}

	if channel, external := journeySideEffectChannel(node.Type); external {
		return planJourneySideEffect(workspaceID, claim, automation, node, channel, now)
	}

	executor, ok := e.nodeExecutors[node.Type]
	if !ok {
		return commit, fmt.Errorf("unsupported claimed journey node type: %s", node.Type)
	}
	var contactData *domain.Contact
	if journeyNodeNeedsContactData(node.Type) {
		contactData, err = e.contactRepo.GetContactByEmail(ctx, workspaceID, next.ContactEmail)
		if err != nil {
			return commit, fmt.Errorf("load contact for claimed journey: %w", err)
		}
	}
	executionContext := make(map[string]interface{})
	if e.automationRepo != nil {
		executionContext, err = e.buildContextFromNodeExecutions(ctx, workspaceID, next.ID)
		if err != nil {
			return commit, fmt.Errorf("build claimed journey context: %w", err)
		}
	}
	result, err := executor.Execute(ctx, NodeExecutionParams{
		WorkspaceID: workspaceID, Contact: &next, Node: node, Automation: automation,
		ContactData: contactData, ExecutionContext: executionContext,
	})
	if err != nil {
		return commit, fmt.Errorf("plan claimed node %s: %w", node.ID, err)
	}
	commit.ContactAutomation.CurrentNodeID = result.NextNodeID
	commit.ContactAutomation.ScheduledAt = result.ScheduledAt
	commit.ContactAutomation.Status = result.Status
	commit.ContactAutomation.ExitReason = result.ExitReason
	if result.Context != nil {
		commit.ContactAutomation.Context = result.Context
	}
	if result.NextNodeID == nil && result.Status == domain.ContactAutomationStatusActive && result.ScheduledAt == nil {
		commit.ContactAutomation.Status = domain.ContactAutomationStatusCompleted
	}
	if commit.ContactAutomation.Status == domain.ContactAutomationStatusActive && commit.ContactAutomation.ScheduledAt == nil {
		commit.ContactAutomation.ScheduledAt = journeyTimePtr(now)
	}
	return commit, nil
}

func journeyNodeNeedsContactData(nodeType domain.NodeType) bool {
	switch nodeType {
	case domain.NodeTypeBranch, domain.NodeTypeFilter, domain.NodeTypeListStatusBranch:
		return true
	default:
		return false
	}
}

func journeySideEffectChannel(nodeType domain.NodeType) (string, bool) {
	switch nodeType {
	case domain.NodeTypeEmail:
		return "email", true
	case domain.NodeTypeAddToList, domain.NodeTypeRemoveFromList:
		return "contact_list", true
	case domain.NodeTypeWebhook:
		return "webhook", true
	default:
		return "", false
	}
}

func planJourneySideEffect(
	workspaceID string,
	claim domain.ContactAutomationClaim,
	automation *domain.Automation,
	node *domain.AutomationNode,
	channel string,
	now time.Time,
) (domain.JourneyStateCommit, error) {
	next := claim.ContactAutomation
	next.CurrentNodeID = node.NextNodeID
	next.ScheduledAt = journeyTimePtr(now)
	if node.NextNodeID == nil {
		next.Status = domain.ContactAutomationStatusCompleted
		next.ScheduledAt = nil
	}

	effectKey := fmt.Sprintf(
		"journey:%s:%s:%d:%s:%d",
		workspaceID, next.ID, claim.AutomationVersion, node.ID, claim.StateVersion,
	)
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(effectKey+":event"))
	messageID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(effectKey+":message"))
	correlationID := eventID
	var causationID *uuid.UUID
	if claim.OriginEventID != nil {
		correlationID = *claim.OriginEventID
		origin := *claim.OriginEventID
		causationID = &origin
	}
	data, err := json.Marshal(map[string]interface{}{
		"contact_automation_id": next.ID,
		"automation_id":         automation.ID,
		"automation_version":    claim.AutomationVersion,
		"state_version":         claim.StateVersion,
		"contact_email":         next.ContactEmail,
		"node_id":               node.ID,
		"node_type":             node.Type,
		"node_config":           node.Config,
	})
	if err != nil {
		return domain.JourneyStateCommit{}, fmt.Errorf("marshal journey side effect data: %w", err)
	}
	envelope := domain.EventEnvelope{
		ID: messageID, EventID: eventID, Type: "journey.side_effect.requested", SchemaVersion: 1,
		WorkspaceID: workspaceID,
		Subject:     domain.EventSubject{Type: "contact_automation", ID: next.ID, ContactEmail: next.ContactEmail},
		Source:      "journey-worker", OccurredAt: now, ReceivedAt: now,
		CorrelationID: correlationID, CausationID: causationID, Data: data,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return domain.JourneyStateCommit{}, fmt.Errorf("marshal journey side effect envelope: %w", err)
	}
	hash := sha256.Sum256(payload)
	requestHash := hex.EncodeToString(hash[:])
	headers, err := json.Marshal(map[string]interface{}{
		"schema_version": 1, "effect_key": effectKey, "channel": channel,
		"correlation_id": correlationID,
	})
	if err != nil {
		return domain.JourneyStateCommit{}, fmt.Errorf("marshal journey side effect headers: %w", err)
	}
	return domain.JourneyStateCommit{
		ContactAutomation: next,
		SideEffect: &domain.SideEffectExecution{
			EffectKey: effectKey, ContactAutomationID: next.ID,
			AutomationVersion: claim.AutomationVersion, NodeID: node.ID,
			ExecutionVersion: claim.StateVersion, Channel: channel,
			Status: domain.SideEffectStatusReserved, RequestHash: requestHash,
			CreatedAt: now, UpdatedAt: now,
		},
		Command: &domain.OutboxMessage{
			ID: messageID, EventID: eventID, Topic: "notifuse.jobs",
			RoutingKey: "delivery." + channel, Payload: payload, Headers: headers,
			Status: domain.OutboxStatusPending, AvailableAt: now, CreatedAt: now,
		},
	}, nil
}

func journeyTimePtr(value time.Time) *time.Time {
	return &value
}

// ProcessBatch processes a batch of scheduled contacts
func (e *AutomationExecutor) ProcessBatch(ctx context.Context, limit int) (int, error) {
	// Get scheduled contacts globally
	contacts, err := e.automationRepo.GetScheduledContactAutomationsGlobal(ctx, time.Now().UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("failed to get scheduled contacts: %w", err)
	}

	if len(contacts) == 0 {
		return 0, nil
	}

	processed := 0
	for _, ca := range contacts {
		if err := e.Execute(ctx, ca.WorkspaceID, &ca.ContactAutomation); err != nil {
			e.logger.WithFields(map[string]interface{}{
				"contact_email": ca.ContactEmail,
				"automation_id": ca.AutomationID,
				"workspace_id":  ca.WorkspaceID,
				"error":         err.Error(),
			}).Error("Failed to execute automation for contact")
			// Continue with other contacts
			continue
		}
		processed++
	}

	return processed, nil
}

// handleError handles an error during execution by updating retry count and status
func (e *AutomationExecutor) handleError(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, err error, context string) error {
	ca.RetryCount++
	errStr := fmt.Sprintf("%s: %s", context, err.Error())
	ca.LastError = &errStr
	now := time.Now().UTC()
	ca.LastRetryAt = &now

	if ca.RetryCount >= ca.MaxRetries {
		ca.Status = domain.ContactAutomationStatusFailed
		_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "failed")

		e.createAutomationEndEvent(ctx, workspaceID, ca, "failed")

		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  workspaceID,
			"retry_count":   ca.RetryCount,
			"error":         errStr,
		}).Error("Automation execution failed after max retries")
	} else {
		// Exponential backoff: 1min, 2min, 4min, etc.
		backoff := time.Duration(1<<uint(ca.RetryCount)) * time.Minute
		nextRetry := time.Now().UTC().Add(backoff)
		ca.ScheduledAt = &nextRetry

		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  workspaceID,
			"retry_count":   ca.RetryCount,
			"next_retry":    nextRetry,
			"error":         errStr,
		}).Warn("Automation execution failed, scheduling retry")
	}

	// Log node execution entry with error
	if ca.CurrentNodeID != nil {
		entry := &domain.NodeExecution{
			ID:                  uuid.NewString(),
			ContactAutomationID: ca.ID,
			AutomationID:        ca.AutomationID,
			NodeID:              *ca.CurrentNodeID,
			NodeType:            domain.NodeTypeTrigger, // Placeholder - actual type not available in error context
			Action:              domain.NodeActionFailed,
			EnteredAt:           time.Now().UTC(),
			Error:               &errStr,
		}
		_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, entry)
	}

	return e.persistIfActive(ctx, workspaceID, ca)
}

// persistIfActive writes the contact automation only while the row is still active, so a
// concurrent stop-on-reply exit (ExitContactJourneysOnReply flips status to 'exited') is
// never clobbered back to active/failed/completed. A 0-row result means the journey was
// exited concurrently — we leave that exit in place and stop, rather than resurrecting it.
func (e *AutomationExecutor) persistIfActive(ctx context.Context, workspaceID string, ca *domain.ContactAutomation) error {
	updated, err := e.automationRepo.UpdateContactAutomationIfActive(ctx, workspaceID, ca)
	if err != nil {
		return err
	}
	if !updated {
		e.logger.WithField("contact_automation_id", ca.ID).
			Info("Contact automation no longer active (exited concurrently); skipping terminal/retry write")
	}
	return nil
}

// markAsCompleted marks a contact automation as completed
func (e *AutomationExecutor) markAsCompleted(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, reason string) error {
	ca.Status = domain.ContactAutomationStatusCompleted
	ca.ScheduledAt = nil
	ca.ExitReason = &reason

	e.logger.WithFields(map[string]interface{}{
		"contact_email": ca.ContactEmail,
		"automation_id": ca.AutomationID,
		"workspace_id":  workspaceID,
		"reason":        reason,
	}).Info("Contact automation completed")

	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "completed")

	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)

	return e.persistIfActive(ctx, workspaceID, ca)
}

// markAsExited marks a contact automation as exited
func (e *AutomationExecutor) markAsExited(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, reason string) error {
	ca.Status = domain.ContactAutomationStatusExited
	ca.ScheduledAt = nil
	ca.ExitReason = &reason

	e.logger.WithFields(map[string]interface{}{
		"contact_email": ca.ContactEmail,
		"automation_id": ca.AutomationID,
		"workspace_id":  workspaceID,
		"reason":        reason,
	}).Info("Contact automation exited")

	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "exited")

	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)

	return e.persistIfActive(ctx, workspaceID, ca)
}

// createNodeExecution creates a new node execution entry for logging
func (e *AutomationExecutor) createNodeExecution(ca *domain.ContactAutomation, node *domain.AutomationNode, action domain.NodeAction) *domain.NodeExecution {
	return &domain.NodeExecution{
		ID:                  uuid.NewString(),
		ContactAutomationID: ca.ID,
		AutomationID:        ca.AutomationID,
		NodeID:              node.ID,
		NodeType:            node.Type,
		Action:              action,
		EnteredAt:           time.Now().UTC(),
		Output:              make(map[string]interface{}),
	}
}

// buildContextFromNodeExecutions reconstructs context from completed node executions
// This allows nodes to access data from previous nodes in the workflow
func (e *AutomationExecutor) buildContextFromNodeExecutions(ctx context.Context, workspaceID, contactAutomationID string) (map[string]interface{}, error) {
	entries, err := e.automationRepo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, entry := range entries {
		if entry.Action == domain.NodeActionCompleted && entry.Output != nil {
			result[entry.NodeID] = entry.Output
		}
	}
	return result, nil
}

// createAutomationEndEvent creates an automation.end timeline event when a contact exits an automation
func (e *AutomationExecutor) createAutomationEndEvent(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, exitReason string) {
	entry := &domain.ContactTimelineEntry{
		Email:      ca.ContactEmail,
		Operation:  "update",
		EntityType: "automation",
		Kind:       "automation.end",
		EntityID:   &ca.AutomationID,
		Changes: map[string]interface{}{
			"automation_id": map[string]interface{}{"new": ca.AutomationID},
			"exit_reason":   map[string]interface{}{"new": exitReason},
			"status":        map[string]interface{}{"new": string(ca.Status)},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := e.timelineRepo.Create(ctx, workspaceID, entry); err != nil {
		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"error":         err.Error(),
		}).Warn("Failed to create automation.end timeline event")
	}
}
