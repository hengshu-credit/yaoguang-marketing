package broadcast

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
)

// queueMessageSender implements the MessageSender interface by enqueueing to the email queue
// instead of sending directly. This allows rate limiting to be handled by the queue workers
// and provides a unified queue for both broadcasts and automations.
type queueMessageSender struct {
	queueRepo           domain.EmailQueueRepository
	deliveryRepo        domain.DeliveryRepository
	frequencyEvaluator  domain.MarketingFrequencyEvaluator
	audienceEligibility AudienceEligibilityChecker
	broadcastRepo       domain.BroadcastRepository
	messageHistoryRepo  domain.MessageHistoryRepository
	templateRepo        domain.TemplateRepository
	dataFeedFetcher     DataFeedFetcher
	logger              logger.Logger
	config              *Config
	apiEndpoint         string
}

const audienceNoLongerMatchedReason = "audience_no_longer_matched"

type AudienceEligibilityChecker interface {
	MatchesCustomerInternal(context.Context, string, string, int, string) (bool, error)
}

// NewQueueMessageSender creates a new message sender that enqueues to the email queue
func NewQueueMessageSender(
	queueRepo domain.EmailQueueRepository,
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	dataFeedFetcher DataFeedFetcher,
	logger logger.Logger,
	config *Config,
	apiEndpoint string,
) MessageSender {
	return newQueueMessageSender(queueRepo, nil, broadcastRepo, messageHistoryRepo, templateRepo, dataFeedFetcher, logger, config, apiEndpoint)
}

// NewQueueMessageSenderWithDelivery enables the crash-safe Delivery Intent
// path while the legacy constructor remains available to compatibility tests
// and direct integrations during the staged rollout.
func NewQueueMessageSenderWithDelivery(
	queueRepo domain.EmailQueueRepository,
	deliveryRepo domain.DeliveryRepository,
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	dataFeedFetcher DataFeedFetcher,
	logger logger.Logger,
	config *Config,
	apiEndpoint string,
) MessageSender {
	return newQueueMessageSender(queueRepo, deliveryRepo, broadcastRepo, messageHistoryRepo, templateRepo, dataFeedFetcher, logger, config, apiEndpoint)
}

func newQueueMessageSender(
	queueRepo domain.EmailQueueRepository,
	deliveryRepo domain.DeliveryRepository,
	broadcastRepo domain.BroadcastRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	dataFeedFetcher DataFeedFetcher,
	log logger.Logger,
	config *Config,
	apiEndpoint string,
) MessageSender {
	if config == nil {
		config = DefaultConfig()
	}

	return &queueMessageSender{
		queueRepo:          queueRepo,
		deliveryRepo:       deliveryRepo,
		broadcastRepo:      broadcastRepo,
		messageHistoryRepo: messageHistoryRepo,
		templateRepo:       templateRepo,
		dataFeedFetcher:    dataFeedFetcher,
		logger:             log,
		config:             config,
		apiEndpoint:        apiEndpoint,
	}
}

// SendToRecipient enqueues a message for a single recipient
func (s *queueMessageSender) SendToRecipient(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	contactLanguage string,
	workspaceDefaultLanguage string,
) error {
	// Build the email payload
	entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, trackingEnabled, webAnalytics, broadcast, messageID, email, template, data, emailProvider, contactLanguage, workspaceDefaultLanguage)
	if err != nil {
		return err
	}

	// Delivery-enabled deployments reserve the logical send and queue row in one
	// transaction. Legacy callers keep the original enqueue path during rollout.
	if s.deliveryRepo != nil {
		customerID, resolveErr := s.deliveryRepo.ResolveCustomerID(ctx, workspaceID, email)
		if resolveErr != nil {
			return NewBroadcastError(ErrCodeSendFailed, "failed to resolve delivery customer", true, resolveErr)
		}
		if strings.TrimSpace(customerID) == "" {
			return NewBroadcastError(ErrCodeSendFailed, "delivery customer authority is missing", false, nil)
		}
		phase := broadcastDeliveryPhase(broadcast)
		if broadcast.Audience.AudienceID != "" {
			entry.Payload.AudienceEligibility = &domain.AudienceEligibilityContext{
				AudienceID: broadcast.Audience.AudienceID, AudienceVersion: broadcast.Audience.AudienceVersion,
				AudienceBuildID: broadcast.Audience.AudienceBuildID, CustomerID: customerID,
			}
		}
		eligible, eligibilityErr := s.checkAudienceEligibility(ctx, workspaceID, broadcast, customerID, phase, "individual", template, 0)
		if eligibilityErr != nil {
			return NewBroadcastError(ErrCodeSendFailed, "failed to evaluate audience eligibility", true, eligibilityErr)
		}
		if !eligible {
			return nil
		}
		if _, reserveErr := s.reserveBroadcastEntry(ctx, workspaceID, broadcast, customerID, email, phase, "individual", template, entry); reserveErr != nil {
			return NewBroadcastError(ErrCodeSendFailed, "failed to reserve delivery", true, reserveErr)
		}
		return nil
	}
	if err := s.queueRepo.Enqueue(ctx, workspaceID, []*domain.EmailQueueEntry{entry}); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"error":        err.Error(),
		}).Error("Failed to enqueue email")
		return NewBroadcastError(ErrCodeSendFailed, "failed to enqueue email", true, err)
	}

	return nil
}

// SendBatch enqueues messages for a batch of recipients
func (s *queueMessageSender) SendBatch(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	websiteURL string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcastID string,
	recipients []*domain.ContactWithList,
	templates map[string]*domain.Template,
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	workspaceDefaultLanguage string,
) (sent int, failed int, err error) {
	if len(recipients) == 0 {
		return 0, 0, nil
	}

	// Get broadcast for context
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return 0, len(recipients), fmt.Errorf("failed to get broadcast: %w", err)
	}

	// Build queue entries
	var entries []*domain.EmailQueueEntry
	var buildErrors int
	var deliveryProcessed int

	for recipientIndex, recipient := range recipients {
		// Check timeout
		if time.Now().After(timeoutAt) {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
			}).Debug("Timeout reached during batch build")
			break
		}

		if recipient == nil || recipient.Contact == nil {
			buildErrors++
			continue
		}

		customerID := recipient.CustomerID
		email := strings.TrimSpace(recipient.Contact.Email)
		phase := recipient.DeliveryPhase
		occurrence := ""
		if s.deliveryRepo != nil {
			if customerID == "" && email != "" {
				customerID, err = s.deliveryRepo.ResolveCustomerID(ctx, workspaceID, email)
				if err != nil {
					return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to resolve delivery customer", true, err)
				}
				if strings.TrimSpace(customerID) == "" {
					return 0, 0, NewBroadcastError(ErrCodeSendFailed, "delivery customer authority is missing", false, nil)
				}
				recipient.CustomerID = customerID
			}
			if phase == "" {
				phase = broadcastDeliveryPhase(broadcast)
				recipient.DeliveryPhase = phase
			}
			ordinal := recipient.SnapshotOrdinal
			if ordinal <= 0 {
				ordinal = int64(recipientIndex + 1)
				recipient.SnapshotOrdinal = ordinal
			}
			occurrence = strconv.FormatInt(ordinal, 10)
		}

		// Delivery-enabled sends freeze the A/B variant deterministically. The
		// legacy path keeps its existing selector until all producers are cut over.
		template := s.selectTemplate(templates, broadcast)
		if s.deliveryRepo != nil {
			if frozen := templates[recipient.DeliveryVariant]; frozen != nil {
				template = frozen
			} else {
				template = selectStableBroadcastTemplate(templates, broadcast, customerID, email, occurrence)
			}
		}
		if template == nil {
			buildErrors++
			continue
		}
		recipient.DeliveryVariant = template.ID
		if s.deliveryRepo != nil {
			eligible, eligibilityErr := s.checkAudienceEligibility(
				ctx, workspaceID, broadcast, customerID, phase, occurrence, template, recipient.SnapshotOrdinal,
			)
			if eligibilityErr != nil {
				return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to evaluate audience eligibility", true, eligibilityErr)
			}
			if !eligible {
				deliveryProcessed++
				continue
			}
		}
		if email == "" {
			if s.deliveryRepo == nil || strings.TrimSpace(customerID) == "" {
				buildErrors++
				continue
			}
			if reserveErr := s.reserveMissingIdentityIntent(ctx, workspaceID, broadcast, customerID, phase, occurrence, template, recipient.SnapshotOrdinal); reserveErr != nil {
				return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to reserve missing-identity delivery", true, reserveErr)
			}
			deliveryProcessed++
			continue
		}

		// Generate message ID
		messageID := fmt.Sprintf("%s_%s", workspaceID, uuid.New().String())
		if s.deliveryRepo != nil {
			effectKey, keyErr := broadcastDeliveryEffectKey(workspaceID, broadcast, customerID, recipient.Contact.Email, phase, occurrence, template.ID)
			if keyErr != nil {
				return 0, 0, keyErr
			}
			messageID = deterministicBroadcastMessageID(workspaceID, effectKey)
		}

		// Default utm_content on a per-recipient copy: the broadcast is shared
		// across recipients and A/B variants differ per recipient, so the shared
		// UTM parameters must never be mutated.
		var utmParams domain.UTMParameters
		if broadcast.UTMParameters != nil {
			utmParams = *broadcast.UTMParameters
		}
		if utmParams.Content == "" {
			utmParams.Content = template.ID
		}

		// Build tracking settings for BuildTemplateData. Deliberately no identity
		// token: BuildTemplateData reads the endpoint and the UTM fields and never
		// looks at IdentifyToken, and the settings the link rewriter sees are built
		// in buildQueueEntry below, which mints this recipient's own token there.
		// Minting a second one here would encrypt a live per-recipient credential
		// once more per contact in the batch loop and park it in a struct nothing
		// reads.
		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:       endpoint,
			EnableTracking: trackingEnabled,
			UTMSource:      utmParams.Source,
			UTMMedium:      utmParams.Medium,
			UTMCampaign:    utmParams.Campaign,
			UTMContent:     utmParams.Content,
			UTMTerm:        utmParams.Term,
			WorkspaceID:    workspaceID,
			MessageID:      messageID,
		}

		// Build template data with all system variables (unsubscribe_url, notification_center_url, etc.)
		req := domain.TemplateDataRequest{
			WorkspaceID:         workspaceID,
			WorkspaceSecretKey:  workspaceSecretKey,
			WorkspaceWebsiteURL: websiteURL,
			ContactWithList:     *recipient,
			MessageID:           messageID,
			TrackingSettings:    trackingSettings,
			Broadcast:           broadcast,
		}
		data, err := domain.BuildTemplateData(req)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    recipient.Contact.Email,
				"error":        err.Error(),
			}).Warn("Failed to build template data")
			buildErrors++
			continue
		}

		// Fetch recipient feed if configured and enabled
		if broadcast.DataFeed != nil && broadcast.DataFeed.RecipientFeed != nil &&
			broadcast.DataFeed.RecipientFeed.Enabled && s.dataFeedFetcher != nil {

			payload := &domain.RecipientFeedRequestPayload{
				Contact:   domain.BuildRecipientFeedContact(recipient.Contact),
				List:      domain.RecipientFeedList{ID: recipient.ListID, Name: recipient.ListName},
				Broadcast: domain.RecipientFeedBroadcast{ID: broadcast.ID, Name: broadcast.Name},
				Workspace: domain.RecipientFeedWorkspace{ID: workspaceID},
			}

			feedData, feedErr := s.dataFeedFetcher.FetchRecipient(ctx, broadcast.DataFeed.RecipientFeed, payload)
			if feedErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcastID,
					"workspace_id": workspaceID,
					"recipient":    recipient.Contact.Email,
					"error":        feedErr.Error(),
				}).Error("Recipient feed fetch failed, pausing broadcast")
				// Return 0,0 — no entries were enqueued (batch Enqueue happens after loop)
				// The broadcast will be paused and the entire batch re-processed on resume
				return 0, 0, fmt.Errorf("%w: recipient feed failed for %s: %v",
					ErrBroadcastShouldPause, recipient.Contact.Email, feedErr)
			}

			data["recipient_feed"] = feedData
		}

		// Extract contact language for variant resolution
		contactLanguage := ""
		if recipient.Contact.Language != nil && !recipient.Contact.Language.IsNull {
			contactLanguage = recipient.Contact.Language.String
		}

		// Build queue entry
		entry, err := s.buildQueueEntry(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, trackingEnabled, webAnalytics, broadcast, messageID, recipient.Contact.Email, template, data, emailProvider, contactLanguage, workspaceDefaultLanguage)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"broadcast_id": broadcastID,
				"workspace_id": workspaceID,
				"recipient":    recipient.Contact.Email,
				"error":        err.Error(),
			}).Warn("Failed to build queue entry")
			buildErrors++
			continue
		}
		if broadcast.Audience.AudienceID != "" {
			entry.Payload.AudienceEligibility = &domain.AudienceEligibilityContext{
				AudienceID: broadcast.Audience.AudienceID, AudienceVersion: broadcast.Audience.AudienceVersion,
				AudienceBuildID: broadcast.Audience.AudienceBuildID, CustomerID: customerID,
			}
		}

		if s.deliveryRepo != nil {
			if _, reserveErr := s.reserveBroadcastEntry(ctx, workspaceID, broadcast, customerID, recipient.Contact.Email, phase, occurrence, template, entry); reserveErr != nil {
				return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to reserve recipient delivery", true, reserveErr)
			}
			deliveryProcessed++
			continue
		}

		entries = append(entries, entry)
	}

	if s.deliveryRepo != nil {
		return deliveryProcessed, buildErrors, nil
	}

	if len(entries) == 0 {
		return 0, buildErrors, nil
	}

	// Enqueue all entries in batch
	if err := s.queueRepo.Enqueue(ctx, workspaceID, entries); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"batch_size":   len(entries),
			"error":        err.Error(),
		}).Error("Failed to enqueue batch")
		// Nothing was written: the enqueue is a single transaction. Reporting
		// the batch as processed (sent+failed) made the orchestrator advance
		// its offset and keyset cursor past recipients that were never
		// enqueued, so they were skipped for good while the broadcast still
		// declared itself processed.
		return 0, 0, NewBroadcastError(ErrCodeSendFailed, "failed to enqueue batch", true, err)
	}

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcastID,
		"workspace_id": workspaceID,
		"enqueued":     len(entries),
		"build_errors": buildErrors,
	}).Debug("Batch enqueued successfully")

	// Return enqueued as "sent" since from the orchestrator's perspective, the job is done
	return len(entries), buildErrors, nil
}

func (s *queueMessageSender) checkAudienceEligibility(
	ctx context.Context,
	workspaceID string,
	broadcast *domain.Broadcast,
	customerID, phase, occurrence string,
	template *domain.Template,
	snapshotOrdinal int64,
) (bool, error) {
	if broadcast == nil || strings.TrimSpace(broadcast.Audience.AudienceID) == "" {
		return true, nil
	}
	if broadcast.Audience.AudienceVersion <= 0 {
		return false, errors.New("resolved audience version is required")
	}
	if s.audienceEligibility == nil {
		return false, errors.New("audience eligibility checker is unavailable")
	}
	if s.deliveryRepo == nil {
		return false, errors.New("delivery repository is required for audience eligibility audit")
	}
	effectKey, err := broadcastDeliveryEffectKey(workspaceID, broadcast, customerID, "", phase, occurrence, template.ID)
	if err != nil {
		return false, err
	}
	existing, err := s.deliveryRepo.GetIntentByEffectKey(ctx, workspaceID, effectKey)
	if err != nil {
		return false, fmt.Errorf("load audience eligibility decision: %w", err)
	}
	if existing != nil && existing.Status == domain.DeliveryStatusSuppressed &&
		existing.SuppressionReason == audienceNoLongerMatchedReason {
		return false, nil
	}

	matches, err := s.audienceEligibility.MatchesCustomerInternal(
		ctx,
		workspaceID,
		broadcast.Audience.AudienceID,
		broadcast.Audience.AudienceVersion,
		customerID,
	)
	if err != nil {
		return false, fmt.Errorf("evaluate current audience facts: %w", err)
	}
	if matches {
		return true, nil
	}

	fingerprint := struct {
		EffectKey       string `json:"effect_key"`
		AudienceID      string `json:"audience_id"`
		AudienceVersion int    `json:"audience_version"`
		AudienceBuildID string `json:"audience_build_id"`
	}{effectKey, broadcast.Audience.AudienceID, broadcast.Audience.AudienceVersion, broadcast.Audience.AudienceBuildID}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return false, fmt.Errorf("encode audience eligibility decision: %w", err)
	}
	digest := sha256.Sum256(encoded)
	intent := domain.DeliveryIntent{
		EffectKey: effectKey, RequestHash: hex.EncodeToString(digest[:]), SourceType: domain.DeliverySourceBroadcast,
		SourceID: broadcast.ID, SourceVersion: broadcastDeliverySourceVersion(broadcast), CustomerID: customerID,
		Channel: "email", TemplateID: template.ID, TemplateVersion: template.Version,
		NodeOrPhase: phase, Occurrence: occurrence, Variant: template.ID, Status: domain.DeliveryStatusSuppressed,
		SuppressionReason: audienceNoLongerMatchedReason,
		Metadata: domain.MapOfAny{
			"audience_id": broadcast.Audience.AudienceID, "audience_version": broadcast.Audience.AudienceVersion,
			"audience_build_id": broadcast.Audience.AudienceBuildID, "snapshot_ordinal": snapshotOrdinal,
		},
	}
	stored, _, err := s.deliveryRepo.ReserveIntent(ctx, workspaceID, intent)
	if err != nil {
		return false, fmt.Errorf("record audience eligibility skip: %w", err)
	}
	if stored.Status != domain.DeliveryStatusSuppressed || stored.SuppressionReason != audienceNoLongerMatchedReason {
		return false, errors.New("audience eligibility effect key belongs to a non-suppressed delivery")
	}
	return false, nil
}

// buildQueueEntry creates an EmailQueueEntry for a recipient
func (s *queueMessageSender) buildQueueEntry(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	trackingEnabled bool,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	contactLanguage string,
	workspaceDefaultLanguage string,
) (*domain.EmailQueueEntry, error) {
	// Default utm_content on a per-recipient copy: the broadcast is shared
	// across recipients and A/B variants differ per recipient, so the shared
	// UTM parameters must never be mutated.
	var utmParams domain.UTMParameters
	if broadcast.UTMParameters != nil {
		utmParams = *broadcast.UTMParameters
	}
	if utmParams.Content == "" {
		utmParams.Content = template.ID
	}

	// This function builds one entry for one recipient, so the token minted
	// here is that recipient's own. A mint failure is logged and the entry is
	// built without it: an unidentified visit costs analytics precision, a
	// dropped entry costs the email.
	identifyToken, identifyAllowedHosts, identifyErr := mintIdentifyToken(email, workspaceSecretKey, webAnalytics)
	if identifyErr != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"workspace_id": workspaceID,
			"recipient":    email,
			"error":        identifyErr.Error(),
		}).Warn("Failed to mint web analytics identity token, enqueueing without it")
	}

	// Build tracking settings
	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:             endpoint,
		EnableTracking:       trackingEnabled,
		UTMSource:            utmParams.Source,
		UTMMedium:            utmParams.Medium,
		UTMCampaign:          utmParams.Campaign,
		UTMContent:           utmParams.Content,
		UTMTerm:              utmParams.Term,
		WorkspaceID:          workspaceID,
		MessageID:            messageID,
		IdentifyToken:        identifyToken,
		IdentifyAllowedHosts: identifyAllowedHosts,
	}

	// Resolve language variant
	emailContent := template.ResolveEmailContent(contactLanguage, workspaceDefaultLanguage)
	if emailContent == nil {
		return nil, fmt.Errorf("email content not available after language resolution")
	}

	// Get sender (use template's sender ID if specified, otherwise default)
	sender := emailProvider.GetSender(emailContent.SenderID)
	if sender == nil {
		return nil, fmt.Errorf("no sender configured for email provider")
	}

	// Compile template with the provided data
	compileReq := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		MessageID:        messageID,
		TemplateData:     data,
		TrackingSettings: trackingSettings,
	}
	// Wires the resolved variant's tree/source + its inbox-preview override.
	emailContent.ApplyToCompileRequest(&compileReq, nil)
	compiledTemplate, err := notifuse_mjml.CompileTemplate(compileReq)
	if err != nil {
		return nil, fmt.Errorf("failed to compile template: %w", err)
	}
	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "template compilation failed"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	htmlContent := *compiledTemplate.HTML

	// Process subject line through Liquid templating
	subject, err := notifuse_mjml.ProcessLiquidTemplate(
		emailContent.Subject,
		data,
		"email_subject",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to process subject: %w", err)
	}

	// Build the queue entry.
	//
	// Note what this stores: htmlContent carries this recipient's nf_id on every
	// allowed link, and Payload is persisted as JSONB in the workspace's
	// email_queue table. The row is deleted when the entry is sent
	// (MarkAsSent) and when the worker gives up on it, but for as long as it
	// waits — a long queue, or a broadcast paused mid-send, whose entries keep
	// status 'paused' until it is resumed or deleted — a credential that stays
	// valid for domain.WebIdentifyTokenTTL sits in the database. Only the
	// TrackingSettings struct itself stays out of storage; the HTML it produced
	// does not.
	entry := &domain.EmailQueueEntry{
		ID:            uuid.New().String(),
		Status:        domain.EmailQueueStatusPending,
		Priority:      domain.EmailQueuePriorityMarketing,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      broadcast.ID,
		IntegrationID: integrationID,
		ProviderKind:  emailProvider.Kind,
		ContactEmail:  email,
		MessageID:     messageID,
		TemplateID:    template.ID,
		Payload: domain.EmailQueuePayload{
			FromAddress:        sender.Email,
			FromName:           sender.Name,
			Subject:            subject,
			HTMLContent:        htmlContent,
			RateLimitPerMinute: emailProvider.RateLimitPerMinute,
			EmailOptions: domain.EmailOptions{
				ReplyTo: emailContent.ReplyTo,
			},
			TemplateVersion: int(template.Version),
			ListID:          broadcast.Audience.List,
			TemplateData:    data, // Store template data for message history
		},
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Extract List-Unsubscribe URL from template data for RFC-8058 compliance (broadcast emails only)
	if unsubscribeURL, ok := data["oneclick_unsubscribe_url"].(string); ok && unsubscribeURL != "" {
		entry.Payload.EmailOptions.ListUnsubscribeURL = unsubscribeURL
	}

	return entry, nil
}

// selectTemplate selects a template for sending
// For A/B testing, this uses random selection; for normal sends, uses the first template
func (s *queueMessageSender) selectTemplate(templates map[string]*domain.Template, broadcast *domain.Broadcast) *domain.Template {
	if len(templates) == 0 {
		return nil
	}

	// If only one template, use it
	if len(templates) == 1 {
		for _, t := range templates {
			return t
		}
	}

	// For A/B testing, randomly select a template
	// Get template IDs in a consistent order
	var templateIDs []string
	for id := range templates {
		templateIDs = append(templateIDs, id)
	}

	// Secure random selection
	n, err := crand.Int(crand.Reader, big.NewInt(int64(len(templateIDs))))
	if err != nil {
		// Fallback to first template if random fails
		return templates[templateIDs[0]]
	}

	return templates[templateIDs[n.Int64()]]
}

func broadcastDeliverySourceVersion(broadcast *domain.Broadcast) string {
	if broadcast != nil && broadcast.Metadata != nil {
		if version, ok := broadcast.Metadata["source_version"].(string); ok && strings.TrimSpace(version) != "" {
			return strings.TrimSpace(version)
		}
	}
	if broadcast != nil && !broadcast.CreatedAt.IsZero() {
		return broadcast.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return "1"
}

func broadcastDeliveryPhase(broadcast *domain.Broadcast) string {
	if broadcast != nil && broadcast.WinningTemplate != nil {
		return "winner"
	}
	if broadcast != nil && broadcast.TestSettings.Enabled {
		return "test"
	}
	return "single"
}

func selectStableBroadcastTemplate(templates map[string]*domain.Template, broadcast *domain.Broadcast, customerID, email, occurrence string) *domain.Template {
	if len(templates) == 0 {
		return nil
	}
	if broadcast != nil && broadcast.WinningTemplate != nil {
		if template := templates[*broadcast.WinningTemplate]; template != nil {
			return template
		}
	}
	if broadcast != nil && !broadcast.TestSettings.Enabled && len(broadcast.TestSettings.Variations) > 0 {
		if template := templates[broadcast.TestSettings.Variations[0].TemplateID]; template != nil {
			return template
		}
	}
	ids := make([]string, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	identity := customerID
	if identity == "" {
		identity = domain.NormalizeEmail(email)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		broadcast.ID, broadcastDeliverySourceVersion(broadcast), identity, occurrence,
	}, "\x00")))
	index := binary.BigEndian.Uint64(sum[:8]) % uint64(len(ids))
	return templates[ids[index]]
}

func broadcastDeliveryEffectKey(workspaceID string, broadcast *domain.Broadcast, customerID, email, phase, occurrence, variant string) (string, error) {
	if broadcast == nil {
		return "", fmt.Errorf("broadcast is required")
	}
	identity := strings.TrimSpace(customerID)
	if identity == "" {
		return "", fmt.Errorf("customer authority id is required for broadcast delivery")
	}
	return (domain.DeliveryEffectKeyInput{
		WorkspaceID: workspaceID, SourceType: string(domain.DeliverySourceBroadcast),
		SourceID: broadcast.ID, SourceVersion: broadcastDeliverySourceVersion(broadcast),
		CustomerID: identity, NodeOrPhase: phase, Occurrence: occurrence, Variant: variant,
	}).EffectKey()
}

func deterministicBroadcastMessageID(workspaceID, effectKey string) string {
	return fmt.Sprintf("%s_%s", workspaceID, uuid.NewSHA1(uuid.NameSpaceOID, []byte(effectKey)).String())
}

func broadcastDeliveryRequestHash(broadcast *domain.Broadcast, customerID, email, phase, occurrence string, template *domain.Template, entry *domain.EmailQueueEntry) (string, error) {
	fingerprint := struct {
		BroadcastID     string                   `json:"broadcast_id"`
		SourceVersion   string                   `json:"source_version"`
		CustomerID      string                   `json:"customer_id,omitempty"`
		Email           string                   `json:"email"`
		Phase           string                   `json:"phase"`
		Occurrence      string                   `json:"occurrence"`
		TemplateID      string                   `json:"template_id"`
		TemplateVersion int64                    `json:"template_version"`
		IntegrationID   string                   `json:"integration_id"`
		ProviderKind    domain.EmailProviderKind `json:"provider_kind"`
		FromAddress     string                   `json:"from_address"`
		FromName        string                   `json:"from_name"`
		Subject         string                   `json:"subject"`
		ReplyTo         string                   `json:"reply_to"`
	}{
		BroadcastID: broadcast.ID, SourceVersion: broadcastDeliverySourceVersion(broadcast),
		CustomerID: customerID, Email: domain.NormalizeEmail(email), Phase: phase, Occurrence: occurrence,
		TemplateID: template.ID, TemplateVersion: template.Version, IntegrationID: entry.IntegrationID,
		ProviderKind: entry.ProviderKind, FromAddress: entry.Payload.FromAddress, FromName: entry.Payload.FromName,
		Subject: entry.Payload.Subject, ReplyTo: entry.Payload.EmailOptions.ReplyTo,
	}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return "", fmt.Errorf("encode broadcast delivery request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (s *queueMessageSender) reserveBroadcastEntry(ctx context.Context, workspaceID string, broadcast *domain.Broadcast, customerID, email, phase, occurrence string, template *domain.Template, entry *domain.EmailQueueEntry) (domain.ReserveDeliveryResult, error) {
	effectKey, err := broadcastDeliveryEffectKey(workspaceID, broadcast, customerID, email, phase, occurrence, template.ID)
	if err != nil {
		return domain.ReserveDeliveryResult{}, err
	}
	requestHash, err := broadcastDeliveryRequestHash(broadcast, customerID, email, phase, occurrence, template, entry)
	if err != nil {
		return domain.ReserveDeliveryResult{}, err
	}
	entry.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("email-queue:"+effectKey)).String()
	intent := domain.DeliveryIntent{
		EffectKey: effectKey, RequestHash: requestHash, SourceType: domain.DeliverySourceBroadcast,
		SourceID: broadcast.ID, SourceVersion: broadcastDeliverySourceVersion(broadcast), CustomerID: customerID,
		Channel: "email", TemplateID: template.ID, TemplateVersion: template.Version,
		NodeOrPhase: phase, Occurrence: occurrence, Variant: template.ID, Status: domain.DeliveryStatusReserved,
		Metadata: domain.MapOfAny{"recipient_email": domain.NormalizeEmail(email)},
	}
	if s.frequencyEvaluator != nil {
		decision, evaluationErr := s.frequencyEvaluator.Evaluate(ctx, domain.FrequencyEvaluationRequest{
			WorkspaceID: workspaceID, CustomerID: customerID, Channel: "email", EffectKey: effectKey,
			CampaignRef: broadcast.ID, OccurredAt: time.Now().UTC(),
		})
		if evaluationErr != nil || decision.Deferred {
			intent.Status = domain.DeliveryStatusDeferred
			intent.SuppressionReason = decision.Reason
			reserved, created, reserveErr := s.deliveryRepo.ReserveIntent(ctx, workspaceID, intent)
			return domain.ReserveDeliveryResult{Intent: reserved, Created: created}, reserveErr
		}
		if !decision.Allowed {
			intent.Status = domain.DeliveryStatusSuppressed
			intent.SuppressionReason = decision.Reason
			reserved, created, reserveErr := s.deliveryRepo.ReserveIntent(ctx, workspaceID, intent)
			return domain.ReserveDeliveryResult{Intent: reserved, Created: created}, reserveErr
		}
	}
	return s.deliveryRepo.ReserveAndEnqueue(ctx, workspaceID, intent, entry)
}

func (s *queueMessageSender) reserveMissingIdentityIntent(ctx context.Context, workspaceID string, broadcast *domain.Broadcast, customerID, phase, occurrence string, template *domain.Template, ordinal int64) error {
	effectKey, err := broadcastDeliveryEffectKey(workspaceID, broadcast, customerID, "", phase, occurrence, template.ID)
	if err != nil {
		return err
	}
	requestHash, err := broadcastDeliveryRequestHash(broadcast, customerID, "", phase, occurrence, template, &domain.EmailQueueEntry{})
	if err != nil {
		return err
	}
	_, _, err = s.deliveryRepo.ReserveIntent(ctx, workspaceID, domain.DeliveryIntent{
		EffectKey: effectKey, RequestHash: requestHash, SourceType: domain.DeliverySourceBroadcast,
		SourceID: broadcast.ID, SourceVersion: broadcastDeliverySourceVersion(broadcast), CustomerID: customerID,
		Channel: "email", TemplateID: template.ID, TemplateVersion: template.Version,
		NodeOrPhase: phase, Occurrence: occurrence, Variant: template.ID, Status: domain.DeliveryStatusSuppressed,
		SuppressionReason: "missing_identity", Metadata: domain.MapOfAny{"snapshot_ordinal": ordinal},
	})
	return err
}
