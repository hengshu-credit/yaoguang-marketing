package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

type configuredDefinitionStub struct {
	domain.BusinessDefinitionRepository
	values map[string]*domain.BusinessDefinition
}

func (s configuredDefinitionStub) Get(_ context.Context, _ string, kind, key string) (*domain.BusinessDefinition, error) {
	d := s.values[kind+":"+key]
	if d == nil {
		return nil, domain.ErrBusinessDefinitionNotFound
	}
	return d, nil
}

func TestConfiguredAutomationRejectsUnknownAndWrongObjectEvents(t *testing.T) {
	defs := configuredDefinitionStub{values: map[string]*domain.BusinessDefinition{
		"object_type:contract": {Status: "active"},
		"event:signed":         {Status: "active", Settings: map[string]any{"subject_type": "contract"}},
	}}
	validator := NewConfigurableAutomationValidator(nil, defs)
	a := &domain.Automation{Trigger: &domain.TimelineTriggerConfig{EventKind: "tracked_event", EventRule: &domain.MarketingEventRule{SubjectType: "contract", Events: []string{"signed"}}}}
	require.NoError(t, validator.ValidateAutomationNodes(context.Background(), "ws", a))
	a.Trigger.EventRule.Events = []string{"unknown"}
	require.ErrorContains(t, validator.ValidateAutomationNodes(context.Background(), "ws", a), "unknown")
	a.Trigger.EventRule.Events = []string{"signed"}
	defs.values["event:signed"].Settings["subject_type"] = "customer"
	require.ErrorContains(t, validator.ValidateAutomationNodes(context.Background(), "ws", a), "subject type")
}

func TestConfiguredAutomationAllowsBuiltinWildcardStateEvents(t *testing.T) {
	defs := configuredDefinitionStub{values: map[string]*domain.BusinessDefinition{"object_type:contract": {Status: "active"}, "event:state.entered": {Status: "active", Settings: map[string]any{"subject_type": "any", "builtin": true}}}}
	a := &domain.Automation{Trigger: &domain.TimelineTriggerConfig{EventKind: "tracked_event", EventRule: &domain.MarketingEventRule{SubjectType: "contract", Events: []string{"state.entered"}}}}
	require.NoError(t, NewConfigurableAutomationValidator(nil, defs).ValidateAutomationNodes(context.Background(), "w", a))
}
