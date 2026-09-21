package service

import (
	"context"
	"database/sql"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
	"time"
)

type businessTestAuth struct {
	domain.AuthService
	member *domain.UserWorkspace
}

func (a businessTestAuth) AuthenticateUserForWorkspace(ctx context.Context, w string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	return ctx, &domain.User{}, a.member, nil
}

type businessTestDefinitions struct {
	domain.BusinessDefinitionRepository
	defs  map[string]*domain.BusinessDefinition
	reads int
}

func (r *businessTestDefinitions) Get(ctx context.Context, w, kind, key string) (*domain.BusinessDefinition, error) {
	r.reads++
	if d := r.defs[kind+":"+key]; d != nil {
		return d, nil
	}
	return nil, domain.ErrBusinessDefinitionNotFound
}
func (r *businessTestDefinitions) List(ctx context.Context, w string, f domain.BusinessDefinitionFilter) ([]domain.BusinessDefinition, int, error) {
	out := []domain.BusinessDefinition{}
	for _, d := range r.defs {
		if d.Kind == f.Kind && (f.Status == "" || f.Status == d.Status) {
			out = append(out, *d)
		}
	}
	return out, len(out), nil
}
func (r *businessTestDefinitions) Save(ctx context.Context, w string, d *domain.BusinessDefinition) (*domain.BusinessDefinition, error) {
	r.defs[d.Kind+":"+d.Key] = d
	return d, nil
}

type businessTestReceipts struct {
	domain.TrackedEventRepository
	stored *domain.TrackedEventResult
}

func (r businessTestReceipts) FindReceiptByPayload(context.Context, string, string, string) (*domain.TrackedEventResult, error) {
	if r.stored != nil {
		return r.stored, nil
	}
	return nil, sql.ErrNoRows
}
func TestTrackedEventRetryUsesFrozenMappingBeforeDefinitions(t *testing.T) {
	now := time.Now().UTC()
	stored := &domain.TrackedEventResult{ReportID: "original", EventID: "id", ReportedAt: now, Duplicate: true}
	defs := &businessTestDefinitions{}
	s := NewTrackedEventService(businessTestReceipts{stored: stored}, defs, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}}, nil)
	result, err := s.Track(context.Background(), domain.TrackEventRequest{WorkspaceID: "w", Source: "erp", MappingKey: "mapping-now-archived", Event: map[string]any{"external": map[string]any{"id": "original"}}})
	if err != nil || result != stored || defs.reads != 0 {
		t.Fatalf("retry reinterpreted mapping: %v reads=%d", err, defs.reads)
	}
}
func TestBusinessConfigContactWriteCannotEditSchema(t *testing.T) {
	auth := businessTestAuth{member: &domain.UserWorkspace{Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}}}}
	s := NewBusinessConfigService(nil, auth)
	_, err := s.Save(context.Background(), "w", &domain.BusinessDefinition{})
	var permission *domain.PermissionError
	if !errors.As(err, &permission) {
		t.Fatalf("expected permission error got %v", err)
	}
}
func TestBusinessConfigPreviewAttributeAndMappedEvent(t *testing.T) {
	s := NewBusinessConfigService(&businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"event:signed": {Kind: "event", Key: "signed", Name: "Signed", Status: "active", Settings: map[string]any{"expiry_mode": "never"}}}}, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	result, err := s.Preview(context.Background(), "w", &domain.BusinessDefinition{Kind: "customer_attribute", Key: "age", Name: "Age", Schema: map[string]any{"type": "integer", "minimum": float64(18)}}, map[string]any{"value": float64(17)})
	if err != nil || result.Valid {
		t.Fatalf("preview ignored scalar validation %v", err)
	}
	m := &domain.BusinessDefinition{Kind: "event_mapping", Key: "external", Name: "External", Settings: map[string]any{"event_code": "signed", "fields": map[string]any{"report_id": "$.id", "triggered_at": "$.at", "data": "$.data", "customer_attributes": "$.customer"}}}
	result, err = s.Preview(context.Background(), "w", m, map[string]any{"id": "r1", "at": time.Now().UTC().Format(time.RFC3339), "data": map[string]any{}, "customer": map[string]any{"external_user_id": "x"}})
	if err != nil || !result.Valid || result.NormalizedEvent == nil {
		t.Fatalf("mapped preview failed %+v %v", result, err)
	}
}
func TestTrackedEventSensitiveReadDoesNotMutateStoredEnvelope(t *testing.T) {
	defs := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"customer_attribute:phone": {Kind: "customer_attribute", Key: "phone", Status: "active", Settings: map[string]any{"sensitive": true}}}}
	auth := businessTestAuth{member: &domain.UserWorkspace{Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}}}
	s := NewTrackedEventService(nil, defs, auth, nil)
	original := []domain.TrackedEventResult{{Event: &domain.TrackedEvent{AttributePaths: map[string][]string{"phone": {"$.data.billing.telephone"}}, CustomerAttributes: map[string]any{"phone": "+8613800000000"}, Data: map[string]any{"billing": map[string]any{"telephone": "+8613800000000"}, "phone": "+8613800000000", "amount": float64(12)}}}}
	safe, err := s.redactResults(context.Background(), "w", original)
	if err != nil {
		t.Fatal(err)
	}
	if safe[0].Event.CustomerAttributes["phone"] != "[redacted]" || safe[0].Event.Data["phone"] != "[redacted]" {
		t.Fatal("sensitive field leaked")
	}
	if safe[0].Event.Data["billing"].(map[string]any)["telephone"] != "[redacted]" {
		t.Fatal("frozen mapping alias leaked sensitive attribute")
	}
	if original[0].Event.CustomerAttributes["phone"] != "+8613800000000" || safe[0].Event.Data["amount"] != float64(12) {
		t.Fatal("redaction damaged stored/unrelated values")
	}
	original[0].Event.Data["groups"] = []any{[]any{map[string]any{"phone": "secret"}}}
	safe, err = s.redactResults(context.Background(), "w", original)
	if err != nil {
		t.Fatal(err)
	}
	if safe[0].Event.Data["groups"].([]any)[0].([]any)[0].(map[string]any)["phone"] != "[redacted]" {
		t.Fatal("nested array sensitive value leaked")
	}
}

type previewCustomerRepository struct {
	domain.CustomerRepository
	locator domain.CustomerLocator
}

func (r *previewCustomerRepository) Get(_ context.Context, _ string, l domain.CustomerLocator) (*domain.Customer, error) {
	r.locator = l
	return &domain.Customer{ID: "canonical-customer"}, nil
}
func TestBusinessConfigPreviewCustomIdentity(t *testing.T) {
	defs := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"customer_attribute:member_number": {Kind: "customer_attribute", Key: "member_number", Status: "active", Schema: map[string]any{"type": "string"}, Settings: map[string]any{"identity": true, "normalization": "lowercase"}}}}
	s := NewBusinessConfigService(defs, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	customers := &previewCustomerRepository{}
	s.SetCustomerRepository(customers)
	d := &domain.BusinessDefinition{Kind: "event", Key: "login", Name: "Login", Settings: map[string]any{"expiry_mode": "never"}}
	result, err := s.Preview(context.Background(), "w", d, map[string]any{"report_id": "one", "event_code": "login", "customer_attributes": map[string]any{"member_number": "  M123  "}, "data": map[string]any{}, "triggered_at": time.Now().Format(time.RFC3339)})
	if err != nil || !result.Valid || result.ResolvedCustomerID != "canonical-customer" {
		t.Fatalf("custom preview %+v %v", result, err)
	}
	if customers.locator.Identity == nil || customers.locator.Identity.Value != "member_number:m123" {
		t.Fatalf("incorrect custom normalization %+v", customers.locator)
	}
}
