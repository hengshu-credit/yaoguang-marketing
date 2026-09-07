package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"net/url"
	"time"
)

type BrowserEventService struct {
	repo        domain.TrackedEventRepository
	definitions domain.BusinessDefinitionRepository
	auth        domain.AuthService
	workspaces  domain.WorkspaceRepository
	customers   domain.CustomerRepository
}

func NewBrowserEventService(repo domain.TrackedEventRepository, defs domain.BusinessDefinitionRepository, auth domain.AuthService, w domain.WorkspaceRepository, c domain.CustomerRepository) *BrowserEventService {
	return &BrowserEventService{repo, defs, auth, w, c}
}

type IssueBrowserCredentialRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	CustomerID  string   `json:"customer_id"`
	SessionID   string   `json:"session_id"`
	Origin      string   `json:"origin"`
	EventCodes  []string `json:"event_codes"`
}

func (s *BrowserEventService) IssueCredential(ctx context.Context, req IssueBrowserCredentialRequest) (string, error) {
	ctx, err := (&TrackedEventService{auth: s.auth}).authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return "", err
	}
	if req.SessionID == "" || len(req.SessionID) > 128 || len(req.EventCodes) == 0 || len(req.EventCodes) > 50 {
		return "", domain.NewValidationError("session_id and 1 to 50 event codes are required")
	}
	origin, err := url.Parse(req.Origin)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return "", domain.NewValidationError("origin must be an exact HTTP origin")
	}
	customer, err := s.customers.Get(ctx, req.WorkspaceID, domain.CustomerLocator{CustomerID: req.CustomerID})
	if err != nil {
		return "", err
	}
	for _, code := range req.EventCodes {
		d, err := s.definitions.Get(ctx, req.WorkspaceID, "event", code)
		if err != nil {
			return "", err
		}
		if err = validateBrowserEventDefinition(d, req.Origin); err != nil {
			return "", err
		}
	}
	workspace, err := s.workspaces.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return "", err
	}
	return domain.SignBrowserEventCredential(domain.BrowserEventClaims{WorkspaceID: req.WorkspaceID, CustomerID: customer.ID, SessionID: req.SessionID, Origin: req.Origin, EventCodes: req.EventCodes, ExpiresAt: time.Now().Add(15 * time.Minute).Unix()}, workspace.Settings.SecretKey)
}
func validateBrowserEventDefinition(d *domain.BusinessDefinition, origin string) error {
	if d.Status != "active" || d.Settings["browser_enabled"] != true || d.Settings["builtin"] == true || d.Settings["authoritative"] == true {
		return domain.NewValidationError("event is not open for browser reporting")
	}
	allowed, ok := d.Settings["allowed_browser_origins"].([]any)
	if !ok {
		return domain.NewValidationError("browser origin is not allowed")
	}
	for _, entry := range allowed {
		if entry == origin {
			return nil
		}
	}
	return domain.NewValidationError("browser origin is not allowed")
}
func (s *BrowserEventService) Track(ctx context.Context, req domain.BrowserTrackEventRequest, origin string) (*domain.TrackedEventResult, error) {
	if req.WorkspaceID == "" || req.SessionID == "" || len(req.SessionID) > 128 || origin == "" {
		return nil, domain.NewValidationError("workspace_id,session_id and browser origin are required")
	}
	workspace, err := s.workspaces.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	code, _ := req.Event["event_code"].(string)
	d, err := s.definitions.Get(ctx, req.WorkspaceID, "event", code)
	if err != nil {
		return nil, err
	}
	if err = validateBrowserEventDefinition(d, origin); err != nil {
		return nil, err
	}
	customerID := ""
	if req.Credential != "" {
		claims, err := domain.VerifyBrowserEventCredential(req.Credential, workspace.Settings.SecretKey, req.WorkspaceID, req.SessionID, origin, time.Now())
		if err != nil {
			return nil, domain.NewValidationError(err.Error())
		}
		allowed := false
		for _, eventCode := range claims.EventCodes {
			if eventCode == code {
				allowed = true
			}
		}
		if !allowed {
			return nil, domain.NewValidationError("credential does not allow this event")
		}
		customer, err := s.customers.Get(ctx, req.WorkspaceID, domain.CustomerLocator{CustomerID: claims.CustomerID})
		if err != nil {
			return nil, err
		}
		customerID = customer.ID
	}
	// Browser caller cannot supply identity, backfill, mapping, source or business object association.
	raw := map[string]any{}
	for k, v := range req.Event {
		switch k {
		case "report_id", "event_code", "event_name", "data", "triggered_at", "expires_at":
			raw[k] = v
		default:
			return nil, domain.NewValidationError("browser event field is not allowed: " + k)
		}
	}
	attrs := map[string]any{"browser_session_id": req.SessionID}
	if customerID != "" {
		attrs["customer_id"] = customerID
	}
	raw["customer_attributes"] = attrs
	copy := *d
	copy.Settings = map[string]any{}
	for k, v := range d.Settings {
		copy.Settings[k] = v
	}
	copy.Settings["subject_type"] = "customer"
	copy.Settings["unmatched_customer"] = "pending"
	e, err := domain.NormalizeTrackedEvent(raw, &copy, nil, time.Now().UTC())
	if err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	if req.TouchpointToken != "" {
		touchpoint, ok := domain.ResolveWebTouchpoint(req.TouchpointToken, workspace.Settings.SecretKey, time.Now())
		if !ok {
			return nil, domain.NewValidationError("invalid touchpoint token")
		}
		e.TrustedTouchpointID = touchpoint
	}
	payload, _ := json.Marshal(struct {
		Session    string
		Origin     string
		Touchpoint string
		Event      map[string]any
	}{req.SessionID, origin, req.TouchpointToken, req.Event})
	hashBytes := sha256.Sum256(payload)
	hash := hex.EncodeToString(hashBytes[:])
	source := "browser:" + origin
	prior, oldHash, err := s.repo.GetReceipt(ctx, req.WorkspaceID, source, e.ReportID)
	if err == nil {
		if oldHash != hash {
			return nil, domain.ErrEventPayloadConflict
		}
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.repo.Ingest(ctx, domain.TrackedEventCommand{WorkspaceID: req.WorkspaceID, WorkspaceSequence: workspace.Sequence, Source: source, PayloadHash: hash, Event: e, Definition: &copy, Attributes: map[string]domain.BusinessDefinition{"browser_session_id": {Key: "browser_session_id", Schema: map[string]any{"type": "string"}, Settings: map[string]any{"identity": false}}}})
}
func (s *BrowserEventService) Identify(ctx context.Context, w, session, credential, origin string) (int, error) {
	workspace, err := s.workspaces.GetByID(ctx, w)
	if err != nil {
		return 0, err
	}
	claims, err := domain.VerifyBrowserEventCredential(credential, workspace.Settings.SecretKey, w, session, origin, time.Now())
	if err != nil {
		return 0, domain.NewValidationError(err.Error())
	}
	customer, err := s.customers.Get(ctx, w, domain.CustomerLocator{CustomerID: claims.CustomerID})
	if err != nil {
		return 0, err
	}
	repo, ok := s.repo.(interface {
		AssociateBrowserSession(context.Context, string, string, string, string, []string) (int, error)
	})
	if !ok {
		return 0, fmt.Errorf("browser association repository unavailable")
	}
	return repo.AssociateBrowserSession(ctx, w, "browser:"+origin, session, customer.ID, claims.EventCodes)
}
