package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type JourneyTraceService struct {
	source domain.JourneyTraceSource
	auth   domain.AuthService
}

func NewJourneyTraceService(source domain.JourneyTraceSource, auth domain.AuthService) (*JourneyTraceService, error) {
	if source == nil {
		return nil, errors.New("journey trace source is required")
	}
	return &JourneyTraceService{source: source, auth: auth}, nil
}

func (s *JourneyTraceService) authenticate(ctx context.Context, workspaceID string) (context.Context, error) {
	if s.auth == nil {
		return ctx, nil
	}
	ctx, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("authenticate journey trace: %w", err)
	}
	if !membership.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeRead, "Insufficient permissions: read access to automations required")
	}
	return ctx, nil
}

func (s *JourneyTraceService) ListInstances(ctx context.Context, request domain.JourneyInstanceListRequest) ([]domain.JourneyInstanceSummary, int, error) {
	if err := request.Validate(); err != nil {
		return nil, 0, err
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID)
	if err != nil {
		return nil, 0, err
	}
	return s.source.ListJourneyInstances(ctx, request)
}

func (s *JourneyTraceService) GetTrace(ctx context.Context, request domain.JourneyTraceRequest) (*domain.JourneyTrace, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	ctx, err := s.authenticate(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return s.source.GetJourneyTrace(ctx, request)
}

type PostgresJourneyTraceSource struct{ workspaces domain.WorkspaceRepository }

func NewPostgresJourneyTraceSource(workspaces domain.WorkspaceRepository) (*PostgresJourneyTraceSource, error) {
	if workspaces == nil {
		return nil, errors.New("workspace repository is required")
	}
	return &PostgresJourneyTraceSource{workspaces: workspaces}, nil
}

func (s *PostgresJourneyTraceSource) resolveCustomerID(ctx context.Context, db *sql.DB, locator domain.JourneyCustomerLocator) (string, error) {
	query := ""
	value := ""
	switch {
	case locator.CustomerID != "":
		query, value = `SELECT COALESCE(merged_into_id, id)::text FROM customers WHERE id::text = $1`, locator.CustomerID
	case locator.CustomerNo != "":
		query, value = `SELECT COALESCE(merged_into_id, id)::text FROM customers WHERE customer_no = $1`, locator.CustomerNo
	case locator.ExternalUserID != "":
		query, value = `SELECT COALESCE(merged_into_id, id)::text FROM customers WHERE external_user_id = $1`, locator.ExternalUserID
	case locator.Email != "":
		query, value = `SELECT COALESCE(customer.merged_into_id, customer.id)::text
			FROM contacts contact JOIN customers customer ON customer.id = contact.customer_id
			WHERE LOWER(contact.email) = LOWER($1) AND contact.deleted_at IS NULL
			ORDER BY contact.updated_at DESC LIMIT 1`, locator.Email
	default:
		return "", domain.ErrJourneyTraceNotFound
	}
	var customerID string
	if err := db.QueryRowContext(ctx, query, value).Scan(&customerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrJourneyTraceNotFound
		}
		return "", err
	}
	return customerID, nil
}

const journeyInstanceSelect = `SELECT
	instance.id::text, instance.enrollment_id::text, COALESCE(instance.contact_automation_id, ''),
	instance.status, COALESCE(instance.current_node_id, ''), COALESCE(instance.waiting_reason, ''),
	instance.next_scheduled_at, instance.started_at, instance.completed_at,
	automation.id, automation.name, customer.id::text, customer.customer_no,
	COALESCE(customer.external_user_id, ''), COALESCE(enrollment.contact_email, ''),
	enrollment.frequency, COALESCE(enrollment.origin_event_id::text, ''),
	COALESCE(decision.decision, 'enrolled'), COALESCE(decision.reason, '')
	FROM journey_instances instance
	JOIN journey_enrollments enrollment ON enrollment.id = instance.enrollment_id
	JOIN automations automation ON automation.id = enrollment.automation_id
	JOIN customers customer ON customer.id = enrollment.customer_id
	LEFT JOIN LATERAL (
		SELECT d.decision, d.reason FROM journey_entry_decisions d
		WHERE d.automation_id = enrollment.automation_id AND d.customer_id = enrollment.customer_id
			AND d.origin_event_id IS NOT DISTINCT FROM enrollment.origin_event_id
		ORDER BY d.decided_at DESC LIMIT 1
	) decision ON TRUE`

type journeyInstanceScanner interface{ Scan(...interface{}) error }

func scanJourneyInstance(scanner journeyInstanceScanner) (domain.JourneyInstanceSummary, error) {
	var result domain.JourneyInstanceSummary
	err := scanner.Scan(
		&result.ID, &result.EnrollmentID, &result.ContactAutomationID,
		&result.Status, &result.CurrentNodeID, &result.WaitingReason,
		&result.NextScheduledAt, &result.StartedAt, &result.CompletedAt,
		&result.AutomationID, &result.AutomationName, &result.CustomerID, &result.CustomerNo,
		&result.ExternalUserID, &result.ContactEmail, &result.Frequency, &result.OriginEventID,
		&result.EntryDecision, &result.EntryReason,
	)
	return result, err
}

func (s *PostgresJourneyTraceSource) ListJourneyInstances(ctx context.Context, request domain.JourneyInstanceListRequest) ([]domain.JourneyInstanceSummary, int, error) {
	db, err := s.workspaces.GetConnection(ctx, request.WorkspaceID)
	if err != nil {
		return nil, 0, err
	}
	customerID, err := s.resolveCustomerID(ctx, db, request.Locator)
	if err != nil {
		return nil, 0, err
	}
	query := journeyInstanceSelect + ` WHERE enrollment.customer_id::text = $1`
	args := []interface{}{customerID}
	if request.AutomationID != "" {
		args = append(args, request.AutomationID)
		query += fmt.Sprintf(" AND enrollment.automation_id = $%d", len(args))
	}
	if request.Status != "" {
		args = append(args, request.Status)
		query += fmt.Sprintf(" AND instance.status = $%d", len(args))
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+query+`) matched`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, request.Limit, request.Offset)
	query += fmt.Sprintf(" ORDER BY instance.started_at DESC, instance.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	instances := make([]domain.JourneyInstanceSummary, 0)
	for rows.Next() {
		instance, scanErr := scanJourneyInstance(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		instances = append(instances, instance)
	}
	return instances, total, rows.Err()
}

func (s *PostgresJourneyTraceSource) GetJourneyTrace(ctx context.Context, request domain.JourneyTraceRequest) (*domain.JourneyTrace, error) {
	db, err := s.workspaces.GetConnection(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	instance, err := scanJourneyInstance(db.QueryRowContext(ctx, journeyInstanceSelect+` WHERE instance.id::text = $1`, request.JourneyInstanceID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrJourneyTraceNotFound
		}
		return nil, err
	}
	trace := &domain.JourneyTrace{Instance: instance, Decisions: []domain.JourneyEntryDecision{}, Events: []domain.JourneyTraceEvent{}, Deliveries: []domain.JourneyDeliveryLink{}}
	decisionRows, err := db.QueryContext(ctx, `SELECT id::text, automation_id, customer_id::text,
		COALESCE(origin_event_id::text, ''), decision, COALESCE(reason, ''), retry_at, decided_at
		FROM journey_entry_decisions WHERE automation_id = $1 AND customer_id::text = $2
			AND origin_event_id IS NOT DISTINCT FROM NULLIF($3, '')::uuid
		ORDER BY decided_at, id`, instance.AutomationID, instance.CustomerID, instance.OriginEventID)
	if err != nil {
		return nil, err
	}
	for decisionRows.Next() {
		var decision domain.JourneyEntryDecision
		if err := decisionRows.Scan(&decision.ID, &decision.AutomationID, &decision.CustomerID, &decision.OriginEventID, &decision.Decision, &decision.Reason, &decision.RetryAt, &decision.DecidedAt); err != nil {
			decisionRows.Close()
			return nil, err
		}
		trace.Decisions = append(trace.Decisions, decision)
	}
	if err := decisionRows.Close(); err != nil {
		return nil, err
	}

	eventRows, err := db.QueryContext(ctx, `SELECT id::text, COALESCE(node_id, ''), event_type, status,
		COALESCE(reason, ''), payload, occurred_at FROM journey_instance_events
		WHERE journey_instance_id::text = $1 ORDER BY occurred_at, id`, request.JourneyInstanceID)
	if err != nil {
		return nil, err
	}
	for eventRows.Next() {
		var event domain.JourneyTraceEvent
		var payload []byte
		if err := eventRows.Scan(&event.ID, &event.NodeID, &event.EventType, &event.Status, &event.Reason, &payload, &event.OccurredAt); err != nil {
			eventRows.Close()
			return nil, err
		}
		_ = json.Unmarshal(payload, &event.Payload)
		trace.Events = append(trace.Events, event)
	}
	if err := eventRows.Close(); err != nil {
		return nil, err
	}

	intentRows, err := db.QueryContext(ctx, `SELECT id::text, effect_key, request_hash, source_type, source_id,
		source_version, COALESCE(customer_id::text, ''), COALESCE(legacy_identity, ''), channel,
		COALESCE(template_id, ''), COALESCE(template_version, 0), node_or_phase, occurrence, variant,
		status, COALESCE(suppression_reason, ''), metadata, created_at, updated_at
		FROM delivery_intents WHERE source_type = 'automation' AND source_id = $1 AND customer_id::text = $2
			AND (metadata->>'journey_instance_id' = $3 OR metadata->>'contact_automation_id' = $4
				OR ($5 <> '' AND occurrence = $5))
		ORDER BY created_at, id`, instance.AutomationID, instance.CustomerID, instance.ID, instance.ContactAutomationID, instance.OriginEventID)
	if err != nil {
		return nil, err
	}
	for intentRows.Next() {
		var link domain.JourneyDeliveryLink
		var metadata []byte
		if err := intentRows.Scan(&link.Intent.ID, &link.Intent.EffectKey, &link.Intent.RequestHash, &link.Intent.SourceType,
			&link.Intent.SourceID, &link.Intent.SourceVersion, &link.Intent.CustomerID, &link.Intent.LegacyIdentity,
			&link.Intent.Channel, &link.Intent.TemplateID, &link.Intent.TemplateVersion, &link.Intent.NodeOrPhase,
			&link.Intent.Occurrence, &link.Intent.Variant, &link.Intent.Status, &link.Intent.SuppressionReason,
			&metadata, &link.Intent.CreatedAt, &link.Intent.UpdatedAt); err != nil {
			intentRows.Close()
			return nil, err
		}
		_ = json.Unmarshal(metadata, &link.Intent.Metadata)
		attemptRows, queryErr := db.QueryContext(ctx, `SELECT id::text, intent_id::text, attempt_no, provider, request_hash,
			COALESCE(provider_message_id, ''), status, COALESCE(claim_token::text, ''), lease_expires_at,
			submitted_at, accepted_at, completed_at, COALESCE(error_category, ''), COALESCE(error_code, ''),
			COALESCE(error_detail, ''), created_at, updated_at FROM delivery_attempts WHERE intent_id::text = $1 ORDER BY attempt_no`, link.Intent.ID)
		if queryErr != nil {
			intentRows.Close()
			return nil, queryErr
		}
		for attemptRows.Next() {
			var attempt domain.DeliveryAttempt
			if err := attemptRows.Scan(&attempt.ID, &attempt.IntentID, &attempt.AttemptNo, &attempt.Provider, &attempt.RequestHash,
				&attempt.ProviderMessageID, &attempt.Status, &attempt.ClaimToken, &attempt.LeaseExpiresAt,
				&attempt.SubmittedAt, &attempt.AcceptedAt, &attempt.CompletedAt, &attempt.ErrorCategory,
				&attempt.ErrorCode, &attempt.ErrorDetail, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
				attemptRows.Close()
				intentRows.Close()
				return nil, err
			}
			link.Attempts = append(link.Attempts, attempt)
		}
		attemptRows.Close()
		receiptRows, queryErr := db.QueryContext(ctx, `SELECT provider, receipt_id, COALESCE(provider_message_id, ''),
			payload_hash, received_at FROM delivery_receipts WHERE effect_key = $1
				OR provider_message_id IN (SELECT provider_message_id FROM delivery_attempts WHERE intent_id::text = $2 AND provider_message_id IS NOT NULL)
			ORDER BY received_at`, link.Intent.EffectKey, link.Intent.ID)
		if queryErr != nil {
			intentRows.Close()
			return nil, queryErr
		}
		for receiptRows.Next() {
			var receipt domain.DeliveryReceiptLink
			if err := receiptRows.Scan(&receipt.Provider, &receipt.ReceiptID, &receipt.ProviderMessageID, &receipt.PayloadHash, &receipt.ReceivedAt); err != nil {
				receiptRows.Close()
				intentRows.Close()
				return nil, err
			}
			receipt.ID = receipt.Provider + ":" + receipt.ReceiptID
			receipt.IntentID = link.Intent.ID
			for _, attempt := range link.Attempts {
				if attempt.Provider == receipt.Provider && attempt.ProviderMessageID == receipt.ProviderMessageID {
					receipt.AttemptID = attempt.ID
					break
				}
			}
			link.Receipts = append(link.Receipts, receipt)
		}
		receiptRows.Close()
		trace.Deliveries = append(trace.Deliveries, link)
	}
	if err := intentRows.Close(); err != nil {
		return nil, err
	}
	return trace, nil
}

var _ domain.JourneyTraceSource = (*PostgresJourneyTraceSource)(nil)
var _ domain.JourneyTraceReader = (*JourneyTraceService)(nil)
