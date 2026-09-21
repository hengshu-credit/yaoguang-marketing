package service

import (
	"context"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBusinessConfigPreviewValidatesObjectIDWithoutWriting(t *testing.T) {
	repo := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{
		"object_type:order": {Kind: "object_type", Key: "order", Name: "Order", Status: "active", Schema: map[string]any{"type": "object"}},
	}}
	s := NewBusinessConfigService(repo, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	d := &domain.BusinessDefinition{Kind: "event", Key: "order.paid", Name: "Paid", Status: "draft", Settings: map[string]any{"subject_type": "order", "subject_id_path": "$.data.order_id", "expiry_mode": "never"}}
	sample := map[string]any{"event_code": d.Key, "report_id": "preview", "customer_attributes": map[string]any{"external_user_id": "customer"}, "data": map[string]any{}, "triggered_at": time.Now().Format(time.RFC3339)}
	result, err := s.Preview(context.Background(), "ws", d, sample)
	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, "$.data.order_id", result.Errors[0].Field)
	sample["data"].(map[string]any)["order_id"] = "order-1"
	result, err = s.Preview(context.Background(), "ws", d, sample)
	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Len(t, repo.defs, 1, "preview must not persist the draft")
}

func TestBusinessConfigDraftCanBeIncompleteButPreviewCannot(t *testing.T) {
	s := NewBusinessConfigService(&businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{}}, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	d := &domain.BusinessDefinition{Kind: "event", Key: "login", Name: "Login", Status: "draft", Settings: map[string]any{"expiry_mode": "duration"}}
	_, err := s.Save(context.Background(), "ws", d)
	require.NoError(t, err)
	result, err := s.Preview(context.Background(), "ws", d, map[string]any{})
	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, "settings.ttl_seconds", result.Errors[0].Field)
	d.Status = "active"
	_, err = s.Save(context.Background(), "ws", d)
	var fieldError *domain.BusinessConfigurationError
	require.ErrorAs(t, err, &fieldError)
}

func TestBusinessConfigMappedEventRequiresActiveNonNativeDefinition(t *testing.T) {
	repo := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{"event:login": {Kind: "event", Key: "login", Status: "draft"}}}
	s := NewBusinessConfigService(repo, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	mapping := &domain.BusinessDefinition{Kind: "event_mapping", Key: "external", Name: "External", Status: "active", Settings: map[string]any{
		"event_code": "login", "fields": map[string]any{"report_id": "$.id", "data": "$.payload", "triggered_at": "$.time", "customer_attributes": "$.customer", "expires_at": "$.expiry"},
	}}
	_, err := s.Save(context.Background(), "ws", mapping)
	require.ErrorContains(t, err, "settings.event_code")
	repo.defs["event:login"].Status = "active"
	delete(mapping.Settings["fields"].(map[string]any), "expires_at")
	_, err = s.Save(context.Background(), "ws", mapping)
	require.ErrorContains(t, err, "settings.fields.expires_at")
	mapping.Settings["fields"].(map[string]any)["expires_at"] = "$.expiry"
	_, err = s.Save(context.Background(), "ws", mapping)
	require.NoError(t, err)
}
