package queue

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/observability"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/emailerror"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
)

// EmailQueueWorkerConfig holds configuration for the worker pool
type EmailQueueWorkerConfig struct {
	WorkerCount   int           // Number of concurrent workers per workspace (default: 5)
	PollInterval  time.Duration // How often to poll for new work (default: 1s)
	BatchSize     int           // How many emails to fetch per poll (default: 50)
	MaxRetries    int           // Max retry attempts before permanent failure (default: 3)
	LeaseDuration time.Duration // Ownership lease for an atomically claimed batch

	// Circuit breaker settings
	CircuitBreakerThreshold int           // Provider errors before opening circuit (default: 5)
	CircuitBreakerCooldown  time.Duration // Time before auto-reset attempt (default: 1 minute)
}

// DefaultWorkerConfig returns sensible default configuration
func DefaultWorkerConfig() *EmailQueueWorkerConfig {
	return &EmailQueueWorkerConfig{
		WorkerCount:             5,
		PollInterval:            1 * time.Second,
		BatchSize:               50,
		MaxRetries:              3,
		LeaseDuration:           2 * time.Minute,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  getCircuitBreakerCooldown(),
	}
}

// EmailSentCallback is called when an email is successfully sent
type EmailSentCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string)

// EmailFailedCallback is called when an email fails to send
type EmailFailedCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isPermanent bool)

// EmailQueueWorker processes queued emails
type EmailQueueWorker struct {
	queueRepo           domain.EmailQueueRepository
	workspaceRepo       domain.WorkspaceRepository
	emailService        domain.EmailServiceInterface
	messageHistoryRepo  domain.MessageHistoryRepository
	deliveryRepo        domain.DeliveryRepository
	audienceEligibility AudienceEligibilityChecker
	// automationRepo is optional; when set, the worker performs the stop-on-reply
	// just-in-time guard before sending automation emails flagged with a
	// contact_automation_id. Injected via SetAutomationRepo (kept out of the
	// constructor to avoid churning every caller).
	automationRepo  domain.AutomationRepository
	rateLimiter     *IntegrationRateLimiter
	circuitBreaker  *IntegrationCircuitBreaker
	errorClassifier *emailerror.Classifier
	config          *EmailQueueWorkerConfig
	logger          logger.Logger

	// Control
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	// Callbacks for progress tracking
	onEmailSent   EmailSentCallback
	onEmailFailed EmailFailedCallback
}

const audienceNoLongerMatchedReason = "audience_no_longer_matched"

type AudienceEligibilityChecker interface {
	MatchesCustomerInternal(context.Context, string, string, int, string) (bool, error)
}

type deliveryIntentSuppressor interface {
	SuppressIntent(context.Context, string, string, domain.DeliveryStatus, string, time.Time) (bool, error)
}

// NewEmailQueueWorker creates a new EmailQueueWorker
func NewEmailQueueWorker(
	queueRepo domain.EmailQueueRepository,
	workspaceRepo domain.WorkspaceRepository,
	emailService domain.EmailServiceInterface,
	messageHistoryRepo domain.MessageHistoryRepository,
	config *EmailQueueWorkerConfig,
	log logger.Logger,
) *EmailQueueWorker {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	// Setup circuit breaker config with defaults
	cbConfig := CircuitBreakerConfig{
		Threshold:      config.CircuitBreakerThreshold,
		CooldownPeriod: config.CircuitBreakerCooldown,
	}
	if cbConfig.Threshold == 0 {
		cbConfig.Threshold = 5
	}
	if cbConfig.CooldownPeriod == 0 {
		cbConfig.CooldownPeriod = getCircuitBreakerCooldown()
	}

	return &EmailQueueWorker{
		queueRepo:          queueRepo,
		workspaceRepo:      workspaceRepo,
		emailService:       emailService,
		messageHistoryRepo: messageHistoryRepo,
		rateLimiter:        NewIntegrationRateLimiter(),
		circuitBreaker:     NewIntegrationCircuitBreaker(cbConfig),
		errorClassifier:    emailerror.NewClassifier(),
		config:             config,
		logger:             log,
	}
}

// SetCallbacks sets callback functions for progress tracking
func (w *EmailQueueWorker) SetCallbacks(onSent EmailSentCallback, onFailed EmailFailedCallback) {
	w.onEmailSent = onSent
	w.onEmailFailed = onFailed
}

// Start begins processing queued emails
func (w *EmailQueueWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.running = true
	w.mu.Unlock()

	w.logger.WithFields(map[string]interface{}{
		"worker_count":  w.config.WorkerCount,
		"poll_interval": w.config.PollInterval.String(),
		"batch_size":    w.config.BatchSize,
	}).Info("Starting email queue worker")

	// Start the main processing loop
	w.wg.Add(1)
	go w.processLoop()

	return nil
}

// Stop gracefully stops all workers
func (w *EmailQueueWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.cancel()
	w.mu.Unlock()

	w.logger.Info("Stopping email queue worker...")
	w.wg.Wait()
	w.logger.Info("Email queue worker stopped")
}

// IsRunning returns whether the worker is currently running
func (w *EmailQueueWorker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// processLoop is the main processing loop that polls for work
func (w *EmailQueueWorker) processLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processAllWorkspaces()
		}
	}
}

// processAllWorkspaces processes pending emails from all workspaces
func (w *EmailQueueWorker) processAllWorkspaces() {
	// Get list of all workspaces
	workspaces, err := w.workspaceRepo.List(w.ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces")
		return
	}

	// Process each workspace concurrently
	var processWg sync.WaitGroup
	semaphore := make(chan struct{}, w.config.WorkerCount)

	for _, workspace := range workspaces {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		semaphore <- struct{}{}
		processWg.Add(1)

		go func(ws *domain.Workspace) {
			defer processWg.Done()
			defer func() { <-semaphore }()

			w.processWorkspace(ws)
		}(workspace)
	}

	processWg.Wait()
}

// processWorkspace processes pending emails for a single workspace
func (w *EmailQueueWorker) processWorkspace(workspace *domain.Workspace) {
	// Calculate dynamic batch size based on rate limit
	// Use 45 seconds as time budget (leave 15s buffer for shutdown)
	minRate := w.getMinEmailRateLimit(workspace)
	effectiveBatchSize := (minRate * 45) / 60 // 75% of what we can send in 1 minute
	if effectiveBatchSize < 1 {
		effectiveBatchSize = 1
	}
	if effectiveBatchSize > w.config.BatchSize {
		effectiveBatchSize = w.config.BatchSize
	}

	// Delivery-aware workers claim and lease in one statement. The legacy fetch
	// path remains available to embedders that have not enabled the ledger yet.
	var entries []*domain.EmailQueueEntry
	var err error
	if w.deliveryRepo != nil {
		lease := w.config.LeaseDuration
		if lease <= 0 {
			lease = 2 * time.Minute
		}
		entries, err = w.queueRepo.ClaimPending(w.ctx, workspace.ID, effectiveBatchSize, lease)
	} else {
		entries, err = w.queueRepo.FetchPending(w.ctx, workspace.ID, effectiveBatchSize)
	}
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"workspace_id": workspace.ID,
			"error":        err.Error(),
		}).Error("Failed to fetch pending emails")
		return
	}

	if len(entries) == 0 {
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"workspace_id": workspace.ID,
		"count":        len(entries),
	}).Debug("Processing queued emails")

	// Process each entry
	for _, entry := range entries {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.processEntry(workspace, entry)
	}
}

// processEntry processes a single queue entry
// SetAutomationRepo injects the automation repository used by the stop-on-reply
// just-in-time guard. Optional; when unset the guard is skipped.
func (w *EmailQueueWorker) SetAutomationRepo(repo domain.AutomationRepository) {
	w.automationRepo = repo
}

// SetDeliveryRepository enables the durable Delivery Intent worker path. It is
// a setter to keep existing embedders source compatible during the rollout.
func (w *EmailQueueWorker) SetDeliveryRepository(repo domain.DeliveryRepository) {
	w.deliveryRepo = repo
}

func (w *EmailQueueWorker) SetAudienceEligibilityChecker(checker AudienceEligibilityChecker) {
	w.audienceEligibility = checker
}

func (w *EmailQueueWorker) processEntry(workspace *domain.Workspace, entry *domain.EmailQueueEntry) {
	claimed := entry.ClaimToken != ""
	deliveryEnabled := claimed && w.deliveryRepo != nil && entry.DeliveryIntentID != ""
	// Get the integration to retrieve the email provider (needed for circuit breaker check)
	integration := workspace.GetIntegrationByID(entry.IntegrationID)
	if integration == nil {
		// Mark as processing first to increment attempts, then handle error
		if !claimed {
			if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
				w.logger.WithFields(map[string]interface{}{
					"entry_id": entry.ID,
					"error":    err.Error(),
				}).Warn("Failed to mark entry as processing")
				return
			}
		}
		w.handleError(workspace, entry, fmt.Errorf("integration not found: %s", entry.IntegrationID), nil)
		return
	}

	// Check circuit breaker before starting a provider attempt. A claimed row is
	// released through its ownership token so another worker cannot race it.
	if w.circuitBreaker.IsOpen(entry.IntegrationID) {
		w.logger.WithFields(map[string]interface{}{
			"entry_id":       entry.ID,
			"integration_id": entry.IntegrationID,
		}).Debug("Circuit breaker open, scheduling retry without provider submission")

		nextRetry := time.Now().Add(w.circuitBreaker.GetConfig().CooldownPeriod)
		var retryErr error
		if claimed {
			retryErr = w.queueRepo.FailClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, "provider circuit breaker is open", &nextRetry)
		} else {
			retryErr = w.queueRepo.SetNextRetry(w.ctx, workspace.ID, entry.ID, nextRetry)
		}
		if retryErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    retryErr.Error(),
			}).Warn("Failed to defer entry for open circuit breaker")
		}
		return
	}

	// Mark as processing (this increments attempts)
	if !claimed {
		if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Warn("Failed to mark entry as processing, may be processed by another worker")
			return
		}
	}

	// Wait for rate limiter - always use current integration rate limit (not stale payload value)
	ratePerMinute := integration.EmailProvider.RateLimitPerMinute
	if ratePerMinute <= 0 {
		ratePerMinute = 60 // Default to 1 per second if not configured
	}

	if err := w.rateLimiter.Wait(w.ctx, entry.IntegrationID, ratePerMinute); err != nil {
		// Context cancelled, don't mark as failed
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Debug("Rate limit wait cancelled")
		return
	}

	// Build the send request
	request := entry.Payload.ToSendEmailProviderRequest(
		workspace.ID,
		entry.IntegrationID,
		entry.MessageID,
		entry.ContactEmail,
		&integration.EmailProvider,
	)

	// Stop-on-reply for capture providers (e.g. SES, which overwrites the Message-ID): give
	// the send a place to write the provider-returned MessageId so we can store the
	// recipient-visible Message-ID post-send. Only the captured value is meaningful here.
	var capturedMessageID string
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderCapturesMessageID(entry.ProviderKind) {
		request.CapturedMessageID = &capturedMessageID
	}

	// Stop-on-reply just-in-time guard: for automation sends flagged with a
	// contact_automation_id, re-check the journey is still active right before
	// sending. If a reply exited it after this email was enqueued, cancel the send.
	//
	// This check belongs HERE, at the actual provider call, and moving it earlier
	// reopens the hole it exists to close. Stopping a journey has two async windows:
	// a contact asleep in a delay, which the event interrupt closes by flipping the
	// journey status so the scheduler stops picking it up; and an email already
	// sitting in this queue, which nothing upstream can recall. The email node only
	// enqueues, so a guard there proves nothing about the moment of sending. The
	// third layer is the optimistic lock on the journey persist, which stops the
	// executor writing over an exit that landed mid-tick. Remove any one and
	// "exited" stops meaning no further mail goes out.
	if w.automationRepo != nil && entry.Payload.ContactAutomationID != nil {
		ca, lookupErr := w.automationRepo.GetContactAutomation(w.ctx, workspace.ID, *entry.Payload.ContactAutomationID)
		if lookupErr != nil {
			// Fail open (send) but make it observable — a silent allow could mask a
			// systematically broken guard.
			w.logger.WithFields(map[string]interface{}{
				"entry_id":              entry.ID,
				"contact_automation_id": *entry.Payload.ContactAutomationID,
				"error":                 lookupErr.Error(),
			}).Warn("Stop-on-reply JIT guard lookup failed; proceeding with send (fail-open)")
		} else if ca != nil && ca.Status != domain.ContactAutomationStatusActive {
			w.logger.WithFields(map[string]interface{}{
				"entry_id":              entry.ID,
				"contact_automation_id": *entry.Payload.ContactAutomationID,
				"status":                string(ca.Status),
			}).Info("Skipping automation email: journey no longer active (stop-on-reply)")
			// Complete the owned row without sending. Delivery-backed automation
			// entries also move their logical intent to cancelled.
			if deliveryEnabled {
				_, _ = w.deliveryRepo.TransitionIntent(w.ctx, workspace.ID, entry.DeliveryIntentID, domain.DeliveryStatusQueued, domain.DeliveryStatusCancelled, time.Now().UTC())
			}
			var delErr error
			if claimed {
				delErr = w.queueRepo.CompleteClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, time.Now().UTC())
			} else {
				delErr = w.queueRepo.MarkAsSent(w.ctx, workspace.ID, entry.ID)
			}
			if delErr != nil {
				w.logger.WithFields(map[string]interface{}{
					"entry_id": entry.ID,
					"error":    delErr.Error(),
				}).Warn("Failed to complete cancelled (stop-on-reply) entry")
			}
			return
		}
	}

	// Final Audience guard: this runs after rate limiting and immediately before
	// the delivery intent moves to submitting/provider I/O. A customer who became
	// ineligible while the message waited in the queue is suppressed here.
	if eligibility := entry.Payload.AudienceEligibility; eligibility != nil {
		if w.audienceEligibility == nil {
			w.deferAudienceEligibilityCheck(workspace, entry, errors.New("audience eligibility checker is unavailable"))
			return
		}
		matches, eligibilityErr := w.audienceEligibility.MatchesCustomerInternal(
			w.ctx, workspace.ID, eligibility.AudienceID, eligibility.AudienceVersion, eligibility.CustomerID,
		)
		if eligibilityErr != nil {
			w.deferAudienceEligibilityCheck(workspace, entry, eligibilityErr)
			return
		}
		if !matches {
			if deliveryEnabled {
				suppressor, ok := w.deliveryRepo.(deliveryIntentSuppressor)
				if !ok {
					w.deferAudienceEligibilityCheck(workspace, entry, errors.New("delivery repository cannot persist audience suppression"))
					return
				}
				updated, suppressErr := suppressor.SuppressIntent(
					w.ctx, workspace.ID, entry.DeliveryIntentID, domain.DeliveryStatusQueued,
					audienceNoLongerMatchedReason, time.Now().UTC(),
				)
				if suppressErr != nil || !updated {
					if suppressErr == nil {
						suppressErr = errors.New("audience suppression transition was rejected")
					}
					w.deferAudienceEligibilityCheck(workspace, entry, suppressErr)
					return
				}
			}
			if err := w.completeAudienceSuppressedEntry(workspace, entry); err != nil {
				w.logger.WithFields(map[string]interface{}{"entry_id": entry.ID, "error": err.Error()}).
					Warn("Failed to complete audience-suppressed queue entry")
			}
			return
		}
	}

	var deliveryAttempt *domain.DeliveryAttempt
	if deliveryEnabled {
		leaseExpiresAt := time.Time{}
		if entry.LeaseExpiresAt != nil {
			leaseExpiresAt = *entry.LeaseExpiresAt
		}
		attempt, startErr := w.deliveryRepo.StartAttempt(w.ctx, workspace.ID, domain.DeliveryAttemptStart{
			IntentID: entry.DeliveryIntentID, Provider: string(integration.EmailProvider.Kind),
			ClaimToken: entry.ClaimToken, LeaseExpiresAt: leaseExpiresAt,
		})
		if startErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    startErr.Error(),
			}).Error("Failed to persist submitting state; provider was not called")
			return
		}
		deliveryAttempt = &attempt
		request.IdempotencyKey = attempt.EffectKey
	}

	// Stop-on-reply: persist the matchable message_history row (carrying smtp_message_id)
	// BEFORE the email physically leaves the provider, so a fast inbound reply (e.g. an
	// auto-responder) can always resolve via GetBySMTPMessageID even if it arrives in the
	// window before the post-send upsert. The post-send upsert below preserves the value
	// (ON CONFLICT COALESCE). Only matters for automation sends where we set the Message-ID.
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderSetsOwnMessageID(entry.ProviderKind) {
		if historyErr := w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, "", nil); historyErr != nil && deliveryAttempt != nil {
			w.recordDeliveryFailure(workspace, entry, *deliveryAttempt, historyErr, nil, false)
			return
		}
	}

	// Send the email through the richer submission boundary when available.
	var submission domain.ProviderSubmissionResult
	var err error
	if submitter, ok := w.emailService.(domain.EmailSubmissionService); ok {
		submission, err = submitter.SubmitEmail(w.ctx, *request, true)
	} else {
		err = w.emailService.SendEmail(w.ctx, *request, true)
		if err == nil {
			submission.Accepted = true
			submission.ProviderMessageID = capturedMessageID
		}
	}
	if err != nil {
		// Classify the error
		classifiedErr := w.errorClassifier.Classify(err, integration.EmailProvider.Kind)

		// Log the classification for debugging
		w.logger.WithFields(map[string]interface{}{
			"entry_id":    entry.ID,
			"error_type":  classifiedErr.Type,
			"provider":    classifiedErr.Provider,
			"http_status": classifiedErr.HTTPStatus,
			"retryable":   classifiedErr.Retryable,
			"original":    err.Error(),
		}).Debug("Classified send error")

		// Record failure to circuit breaker (only counts provider errors)
		w.circuitBreaker.RecordFailure(entry.IntegrationID, classifiedErr)

		if deliveryAttempt != nil {
			w.recordDeliveryFailure(workspace, entry, *deliveryAttempt, err, classifiedErr, isUncertainProviderError(err))
		} else {
			w.handleError(workspace, entry, err, classifiedErr)
		}
		return
	}
	if deliveryAttempt != nil && !submission.Accepted {
		providerErr := errors.New("provider returned without an explicit acceptance")
		w.recordDeliveryFailure(workspace, entry, *deliveryAttempt, providerErr, nil, true)
		return
	}

	// Record success to reset circuit breaker
	w.circuitBreaker.RecordSuccess(entry.IntegrationID)
	if deliveryAttempt != nil {
		providerMessageID := submission.ProviderMessageID
		if providerMessageID == "" {
			providerMessageID = capturedMessageID
		}
		acceptedAt := time.Now().UTC()
		if outcomeErr := w.deliveryRepo.RecordAttemptOutcome(w.ctx, workspace.ID, deliveryAttempt.ID, entry.ClaimToken, domain.DeliveryAttemptOutcome{
			Status: domain.DeliveryStatusProviderAccepted, ProviderMessageID: providerMessageID, OccurredAt: acceptedAt,
		}); outcomeErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    outcomeErr.Error(),
			}).Error("Provider accepted email but local accepted state was not persisted; retry is blocked by submitting attempt")
			return
		}
		if historyErr := w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, providerMessageID, nil); historyErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    historyErr.Error(),
			}).Error("Provider accepted email but message history confirmation failed; leaving provider_accepted for reconciliation")
			return
		}
		if outcomeErr := w.deliveryRepo.RecordAttemptOutcome(w.ctx, workspace.ID, deliveryAttempt.ID, entry.ClaimToken, domain.DeliveryAttemptOutcome{
			Status: domain.DeliveryStatusConfirmed, ProviderMessageID: providerMessageID, OccurredAt: time.Now().UTC(),
		}); outcomeErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    outcomeErr.Error(),
			}).Error("Provider accepted email but final confirmation failed; leaving provider_accepted for reconciliation")
			return
		}
		latency := time.Duration(0)
		if deliveryAttempt.SubmittedAt != nil {
			latency = time.Since(*deliveryAttempt.SubmittedAt)
		}
		observability.RecordDeliveryOutcome(w.ctx, "email", string(integration.EmailProvider.Kind), string(domain.DeliveryStatusConfirmed), latency)
		w.logDeliverySuccess(workspace, entry)
		return
	}

	// Complete legacy claimed rows without deleting them; old non-claim callers
	// retain the historical deletion behavior.
	var completeErr error
	if claimed {
		completeErr = w.queueRepo.CompleteClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, time.Now().UTC())
	} else {
		completeErr = w.queueRepo.MarkAsSent(w.ctx, workspace.ID, entry.ID)
	}
	if completeErr != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    completeErr.Error(),
		}).Error("Failed to mark email as sent")
		return
	}

	// Upsert message history (success - clears any previous failure). capturedMessageID
	// carries the SES-returned Message-ID for the reply-matching row (empty for others).
	w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, capturedMessageID, nil)

	w.logger.WithFields(map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"source_type":  entry.SourceType,
		"source_id":    entry.SourceID,
		"workspace_id": workspace.ID,
	}).Debug("Email sent successfully")

	// Call success callback
	if w.onEmailSent != nil {
		w.onEmailSent(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID)
	}
}

func (w *EmailQueueWorker) deferAudienceEligibilityCheck(workspace *domain.Workspace, entry *domain.EmailQueueEntry, cause error) {
	nextRetry := domain.CalculateNextRetryTime(entry.Attempts)
	var err error
	if entry.ClaimToken != "" {
		err = w.queueRepo.FailClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, cause.Error(), &nextRetry)
	} else {
		err = w.queueRepo.MarkAsFailed(w.ctx, workspace.ID, entry.ID, cause.Error(), &nextRetry)
	}
	if err != nil {
		w.logger.WithFields(map[string]interface{}{"entry_id": entry.ID, "error": err.Error()}).
			Error("Failed to defer audience eligibility check")
	}
}

func (w *EmailQueueWorker) completeAudienceSuppressedEntry(workspace *domain.Workspace, entry *domain.EmailQueueEntry) error {
	if entry.ClaimToken != "" {
		return w.queueRepo.CompleteClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, time.Now().UTC())
	}
	return w.queueRepo.MarkAsSent(w.ctx, workspace.ID, entry.ID)
}

// handleError handles a send error, scheduling retry or deleting permanently failed entries
// classifiedErr may be nil for internal errors (e.g., integration not found)
func (w *EmailQueueWorker) handleError(workspace *domain.Workspace, entry *domain.EmailQueueEntry, sendErr error, classifiedErr *emailerror.ClassifiedError) {
	claimed := entry.ClaimToken != ""
	if !claimed {
		entry.Attempts++ // MarkAsProcessing increments the stored value on the legacy path.
	}

	// Determine if this is a permanent failure (non-retryable recipient error or max attempts)
	isPermanent := entry.Attempts >= entry.MaxAttempts
	if classifiedErr != nil && !classifiedErr.Retryable {
		isPermanent = true
	}

	logFields := map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"attempts":     entry.Attempts,
		"max_attempts": entry.MaxAttempts,
		"error":        sendErr.Error(),
		"is_permanent": isPermanent,
	}
	if classifiedErr != nil {
		logFields["error_type"] = classifiedErr.Type
	}
	w.logger.WithFields(logFields).Warn("Failed to send email")

	// Upsert message history with failure info (no captured Message-ID on a failed send).
	w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, "", sendErr)

	if isPermanent {
		// Permanent failure - delete the queue entry
		// Message history already tracks this permanent failure via upsertMessageHistory above
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"attempts":   entry.Attempts,
		}).Warn("Email permanently failed")

		var terminalErr error
		if claimed {
			terminalErr = w.queueRepo.FailClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, sendErr.Error(), nil)
		} else {
			terminalErr = w.queueRepo.Delete(w.ctx, workspace.ID, entry.ID)
		}
		if terminalErr != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    terminalErr.Error(),
			}).Error("Failed to persist permanently failed queue entry")
		}

		// Call failure callback (isPermanent = true)
		if w.onEmailFailed != nil {
			w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, true)
		}
		return
	}

	// Schedule retry with exponential backoff
	nextRetry := domain.CalculateNextRetryTime(entry.Attempts)
	var retryErr error
	if claimed {
		retryErr = w.queueRepo.FailClaim(w.ctx, workspace.ID, entry.ID, entry.ClaimToken, sendErr.Error(), &nextRetry)
	} else {
		retryErr = w.queueRepo.MarkAsFailed(w.ctx, workspace.ID, entry.ID, sendErr.Error(), &nextRetry)
	}
	if retryErr != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    retryErr.Error(),
		}).Error("Failed to mark as failed for retry")
	}

	// Call failure callback (isPermanent = false, will retry)
	if w.onEmailFailed != nil {
		w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, false)
	}
}

func (w *EmailQueueWorker) recordDeliveryFailure(workspace *domain.Workspace, entry *domain.EmailQueueEntry, attempt domain.DeliveryAttempt, sendErr error, classifiedErr *emailerror.ClassifiedError, uncertain bool) {
	status := domain.DeliveryStatusTransientFailed
	if uncertain {
		status = domain.DeliveryStatusUnknown
	} else if entry.Attempts >= entry.MaxAttempts || (classifiedErr != nil && !classifiedErr.Retryable) {
		status = domain.DeliveryStatusTerminalFailed
	}
	var nextRetryAt *time.Time
	if status == domain.DeliveryStatusTransientFailed {
		next := domain.CalculateNextRetryTime(entry.Attempts)
		nextRetryAt = &next
	}
	category, code := "internal", ""
	if classifiedErr != nil {
		category = string(classifiedErr.Type)
		if classifiedErr.HTTPStatus != 0 {
			code = fmt.Sprintf("http_%d", classifiedErr.HTTPStatus)
		}
	}
	outcomeErr := w.deliveryRepo.RecordAttemptOutcome(w.ctx, workspace.ID, attempt.ID, entry.ClaimToken, domain.DeliveryAttemptOutcome{
		Status: status, ErrorCategory: category, ErrorCode: code,
		ErrorDetail: sendErr.Error(), NextRetryAt: nextRetryAt, OccurredAt: time.Now().UTC(),
	})
	if outcomeErr != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"status":   status,
			"error":    outcomeErr.Error(),
		}).Error("Failed to persist delivery attempt outcome; unresolved attempt blocks retry")
		return
	}
	latency := time.Duration(0)
	if attempt.SubmittedAt != nil {
		latency = time.Since(*attempt.SubmittedAt)
	}
	observability.RecordDeliveryOutcome(w.ctx, "email", attempt.Provider, string(status), latency)
	_ = w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, "", sendErr)
	if w.onEmailFailed != nil {
		w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, status != domain.DeliveryStatusTransientFailed)
	}
}

func isUncertainProviderError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"connection reset", "unexpected eof", "broken pipe", "timeout awaiting response", "stream error"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (w *EmailQueueWorker) logDeliverySuccess(workspace *domain.Workspace, entry *domain.EmailQueueEntry) {
	w.logger.WithFields(map[string]interface{}{
		"entry_id": entry.ID, "delivery_intent_id": entry.DeliveryIntentID,
		"message_id": entry.MessageID, "recipient": entry.ContactEmail,
		"source_type": entry.SourceType, "source_id": entry.SourceID,
		"workspace_id": workspace.ID,
	}).Debug("Email delivery confirmed")
	if w.onEmailSent != nil {
		w.onEmailSent(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID)
	}
}

// upsertMessageHistory creates or updates a message history record after a send attempt
// On success: FailedAt and StatusInfo are nil (clears any previous failure)
// On failure: FailedAt is set to now, StatusInfo contains the error
func (w *EmailQueueWorker) upsertMessageHistory(
	ctx context.Context,
	workspaceID string,
	secretKey string,
	entry *domain.EmailQueueEntry,
	capturedMessageID string,
	sendErr error,
) error {
	now := time.Now().UTC()

	message := &domain.MessageHistory{
		ID:              entry.MessageID,
		ContactEmail:    entry.ContactEmail,
		TemplateID:      entry.TemplateID,
		TemplateVersion: int64(entry.Payload.TemplateVersion),
		Channel:         "email",
		MessageData:     domain.MessageData{Data: entry.Payload.TemplateData}, // Include template data for logging
		SentAt:          entry.CreatedAt,                                      // Use queue entry creation time (stable across retries)
		CreatedAt:       entry.CreatedAt,
		UpdatedAt:       now,
	}

	// Set source (broadcast or automation)
	if entry.SourceType == domain.EmailQueueSourceBroadcast {
		message.BroadcastID = &entry.SourceID
		if entry.Payload.ListID != "" {
			message.ListID = &entry.Payload.ListID
		}
	} else if entry.SourceType == domain.EmailQueueSourceAutomation {
		message.AutomationID = &entry.SourceID
	}

	// Record the recipient-visible Message-ID for ALL automation sends on providers
	// where we set the Message-ID ourselves, so an inbound reply's In-Reply-To
	// matches the exact source automation (not just exit_on_reply ones). This is
	// what lets a reply to a non-exit_on_reply automation NOT wrongly exit a
	// different exit_on_reply automation for the same contact.
	if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderSetsOwnMessageID(entry.ProviderKind) {
		// set_own (e.g. Mailgun): store the bracket-free value we set as the header.
		smtpMessageID := domain.RFCMessageIDValue(entry.MessageID, entry.Payload.FromAddress)
		message.SMTPMessageID = &smtpMessageID
	} else if entry.SourceType == domain.EmailQueueSourceAutomation && domain.ProviderCapturesMessageID(entry.ProviderKind) && capturedMessageID != "" {
		// capture (e.g. SES overwrites the Message-ID): store the provider-returned id,
		// reconstructed into the recipient-visible RFC Message-ID value. Only known
		// post-send, so this never runs on the pre-send upsert.
		smtpMessageID := domain.SESStoredMessageID(capturedMessageID)
		message.SMTPMessageID = &smtpMessageID
	}

	// Set failure info if send failed (will be cleared on retry success via UPSERT)
	if sendErr != nil {
		message.FailedAt = &now
		errStr := sendErr.Error()
		if len(errStr) > 255 {
			errStr = errStr[:255]
		}
		message.StatusInfo = &errStr
	}
	// On success: FailedAt and StatusInfo remain nil, clearing any previous failure

	// Upsert record (log errors but don't fail the send operation)
	if err := w.messageHistoryRepo.Upsert(ctx, workspaceID, secretKey, message); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"error":      err.Error(),
		}).Warn("Failed to upsert message history")
		return err
	}
	return nil
}

// GetStats returns statistics about the rate limiters
func (w *EmailQueueWorker) GetStats() map[string]RateLimiterStats {
	return w.rateLimiter.GetStats()
}

// GetConfig returns the worker configuration
func (w *EmailQueueWorker) GetConfig() *EmailQueueWorkerConfig {
	return w.config
}

// GetCircuitBreakerStats returns statistics about all circuit breakers
func (w *EmailQueueWorker) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return w.circuitBreaker.GetStats()
}

// getMinEmailRateLimit returns the minimum rate limit across all email integrations
// Returns default of 60 if no email integrations found
func (w *EmailQueueWorker) getMinEmailRateLimit(workspace *domain.Workspace) int {
	emailIntegrations := workspace.GetIntegrationsByType(domain.IntegrationTypeEmail)
	if len(emailIntegrations) == 0 {
		return 60 // Default: 1 per second
	}

	minRate := emailIntegrations[0].EmailProvider.RateLimitPerMinute
	for _, integration := range emailIntegrations[1:] {
		if integration.EmailProvider.RateLimitPerMinute < minRate {
			minRate = integration.EmailProvider.RateLimitPerMinute
		}
	}
	return minRate
}
