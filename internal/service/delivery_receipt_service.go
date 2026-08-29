package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

var (
	ErrInvalidTwilioSignature    = errors.New("invalid Twilio signature")
	ErrTwilioIntegrationNotFound = errors.New("Twilio integration not found")
)

type DeliveryReceiptService struct {
	auth          domain.AuthService
	repo          domain.DeliveryReceiptRepository
	workspaceRepo domain.WorkspaceRepository
	maxBatchSize  int
	now           func() time.Time
}

func NewDeliveryReceiptService(
	auth domain.AuthService,
	repo domain.DeliveryReceiptRepository,
	workspaceRepo domain.WorkspaceRepository,
	maxBatchSize int,
) (*DeliveryReceiptService, error) {
	if auth == nil || repo == nil || workspaceRepo == nil {
		return nil, errors.New("delivery receipt dependencies are required")
	}
	if maxBatchSize <= 0 {
		return nil, errors.New("delivery receipt batch size must be positive")
	}
	return &DeliveryReceiptService{
		auth: auth, repo: repo, workspaceRepo: workspaceRepo, maxBatchSize: maxBatchSize,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *DeliveryReceiptService) Ingest(
	ctx context.Context,
	request *domain.IngestDeliveryReceiptsRequest,
) (*domain.IngestDeliveryReceiptsResponse, error) {
	if request == nil {
		return nil, domain.NewValidationError("request is required")
	}
	if err := request.ValidateEnvelope(s.maxBatchSize); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	authenticatedCtx, _, userWorkspace, err := s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate delivery receipt request: %w", err)
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceMessageHistory, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceMessageHistory,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to message history required for delivery receipts",
		)
	}

	response := &domain.IngestDeliveryReceiptsResponse{
		Results: make([]domain.IngestDeliveryReceiptResult, len(request.Receipts)),
	}
	validReceipts := make([]domain.DeliveryReceipt, 0, len(request.Receipts))
	validIndices := make([]int, 0, len(request.Receipts))
	for index := range request.Receipts {
		receipt := request.Receipts[index]
		response.Results[index].Provider = receipt.Provider
		response.Results[index].ReceiptID = receipt.ReceiptID
		if err := receipt.Validate(); err != nil {
			response.Results[index].Status = "error"
			response.Results[index].Error = err.Error()
			response.Failed++
			continue
		}
		hash, err := receipt.ComputePayloadHash()
		if err != nil {
			response.Results[index].Status = "error"
			response.Results[index].Error = err.Error()
			response.Failed++
			continue
		}
		receipt.PayloadHash = hash
		validReceipts = append(validReceipts, receipt)
		validIndices = append(validIndices, index)
	}
	if len(validReceipts) == 0 {
		return response, nil
	}

	recorded, err := s.repo.RecordBatch(authenticatedCtx, request.WorkspaceID, validReceipts)
	if err != nil {
		return nil, fmt.Errorf("record delivery receipts: %w", err)
	}
	if len(recorded) != len(validReceipts) {
		return nil, fmt.Errorf("record delivery receipts: repository returned %d results for %d receipts", len(recorded), len(validReceipts))
	}
	for resultIndex, record := range recorded {
		responseIndex := validIndices[resultIndex]
		result := domain.IngestDeliveryReceiptResult{DeliveryReceiptRecordResult: record}
		switch {
		case record.Conflict:
			result.Status = "conflict"
			result.Error = domain.ErrDeliveryReceiptPayloadConflict.Error()
			response.Conflicts++
		case record.Duplicate:
			result.Status = "duplicate"
			response.Duplicates++
		default:
			result.Status = "accepted"
			response.Accepted++
		}
		response.Results[responseIndex] = result
	}
	return response, nil
}

func (s *DeliveryReceiptService) ProcessTwilioCallback(
	ctx context.Context,
	callback domain.TwilioDeliveryCallback,
) (*domain.DeliveryReceiptRecordResult, error) {
	if callback.WorkspaceID == "" || callback.IntegrationID == "" {
		return nil, domain.NewValidationError("workspace_id and integration_id are required")
	}
	if callback.CallbackURL == "" || callback.Signature == "" {
		return nil, ErrInvalidTwilioSignature
	}
	workspace, err := s.workspaceRepo.GetByID(ctx, callback.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load Twilio callback workspace: %w", err)
	}
	integration := workspace.GetIntegrationByID(callback.IntegrationID)
	if integration == nil || integration.Type != domain.IntegrationTypeSMS ||
		integration.SMSProvider == nil || integration.SMSProvider.Kind != domain.SMSProviderKindTwilio ||
		integration.SMSProvider.Twilio == nil || integration.SMSProvider.Twilio.AuthToken == "" {
		return nil, ErrTwilioIntegrationNotFound
	}
	if !validateTwilioSignature(callback.CallbackURL, callback.Form, callback.Signature, integration.SMSProvider.Twilio.AuthToken) {
		return nil, ErrInvalidTwilioSignature
	}

	providerMessageID := firstFormValue(callback.Form, "MessageSid")
	providerStatus := strings.ToLower(strings.TrimSpace(firstFormValue(callback.Form, "MessageStatus")))
	if providerMessageID == "" || providerStatus == "" {
		return nil, domain.NewValidationError("Twilio MessageSid and MessageStatus are required")
	}
	event, err := normalizeTwilioDeliveryEvent(providerStatus)
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	errorCode := strings.TrimSpace(firstFormValue(callback.Form, "ErrorCode"))
	rawDoneAt := strings.TrimSpace(firstFormValue(callback.Form, "RawDlrDoneDate"))
	occurredAt := s.now()
	if parsed, ok := parseTwilioOccurredAt(rawDoneAt); ok {
		occurredAt = parsed
	}
	receiptID := twilioReceiptID(providerMessageID, providerStatus, errorCode, rawDoneAt)
	metadata := map[string]interface{}{"provider_status": providerStatus}
	if rawDoneAt != "" {
		metadata["raw_dlr_done_date"] = rawDoneAt
	}
	receipt := domain.DeliveryReceipt{
		Provider: domain.DeliveryProviderTwilio, ReceiptID: receiptID,
		ProviderMessageID: providerMessageID, MessageID: callback.MessageID, EffectKey: callback.EffectKey,
		Event: event, OccurredAt: occurredAt, ErrorCode: errorCode, Metadata: metadata,
	}
	if err := receipt.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	receipt.PayloadHash, err = receipt.ComputePayloadHash()
	if err != nil {
		return nil, err
	}
	results, err := s.repo.RecordBatch(ctx, callback.WorkspaceID, []domain.DeliveryReceipt{receipt})
	if err != nil {
		return nil, fmt.Errorf("record Twilio delivery receipt: %w", err)
	}
	if len(results) != 1 {
		return nil, errors.New("record Twilio delivery receipt returned no result")
	}
	if results[0].Conflict {
		return &results[0], domain.ErrDeliveryReceiptPayloadConflict
	}
	return &results[0], nil
}

func validateTwilioSignature(callbackURL string, form map[string][]string, signature, authToken string) bool {
	keys := make([]string, 0, len(form))
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var input strings.Builder
	input.WriteString(callbackURL)
	for _, key := range keys {
		values := append([]string(nil), form[key]...)
		sort.Strings(values)
		for _, value := range values {
			input.WriteString(key)
			input.WriteString(value)
		}
	}
	mac := hmac.New(sha1.New, []byte(authToken))
	_, _ = mac.Write([]byte(input.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

func normalizeTwilioDeliveryEvent(status string) (domain.DeliveryReceiptEvent, error) {
	switch status {
	case "accepted", "queued", "scheduled":
		return domain.DeliveryReceiptAccepted, nil
	case "sending", "sent":
		return domain.DeliveryReceiptSent, nil
	case "delivered":
		return domain.DeliveryReceiptDelivered, nil
	case "read":
		return domain.DeliveryReceiptOpened, nil
	case "failed", "undelivered", "canceled":
		return domain.DeliveryReceiptFailed, nil
	default:
		return "", fmt.Errorf("unsupported Twilio MessageStatus %q", status)
	}
}

func parseTwilioOccurredAt(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "0601021504"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func twilioReceiptID(messageSID, status, errorCode, rawDoneAt string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{messageSID, status, errorCode, rawDoneAt}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func firstFormValue(form map[string][]string, key string) string {
	values := form[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
