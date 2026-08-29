package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestCompilerIndexesUpdatedFields(t *testing.T) {
	automation := ruleAutomation("contact.updated")
	automation.Trigger.UpdatedFields = []string{"language", "country", "language"}

	binding, err := NewTriggerBindingCompiler(NewQueryBuilder()).Compile(automation)
	require.NoError(t, err)
	assert.Equal(t, "contact.updated", binding.EventType)
	assert.Equal(t, "contact", binding.SubjectType)
	assert.Equal(t, []string{"changes.country", "changes.language"}, binding.DependencyKeys)
}

func TestCompilerScopesListAndCustomEvents(t *testing.T) {
	listID := "customers"
	listAutomation := ruleAutomation("list.subscribed")
	listAutomation.Trigger.ListID = &listID
	listBinding, err := NewTriggerBindingCompiler(NewQueryBuilder()).Compile(listAutomation)
	require.NoError(t, err)
	assert.Equal(t, "contact_list", listBinding.SubjectType)
	assert.Equal(t, []string{"entity_id:customers"}, listBinding.DependencyKeys)

	eventName := "shop.order"
	customAutomation := ruleAutomation("custom_event")
	customAutomation.Trigger.CustomEventName = &eventName
	customBinding, err := NewTriggerBindingCompiler(NewQueryBuilder()).Compile(customAutomation)
	require.NoError(t, err)
	assert.Equal(t, "custom_event.shop.order", customBinding.EventType)
	assert.Equal(t, "custom_event", customBinding.SubjectType)
}

func TestCompilerStoresParameterizedConditionAndStableHash(t *testing.T) {
	automation := ruleAutomation("contact.updated")
	automation.Trigger.Conditions = &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
				FieldName: "language", FieldType: "string", Operator: "equals", StringValues: []string{"fr"},
			}}},
		},
	}
	compiler := NewTriggerBindingCompiler(NewQueryBuilder())

	first, err := compiler.Compile(automation)
	require.NoError(t, err)
	second, err := compiler.Compile(automation)
	require.NoError(t, err)
	assert.Equal(t, first.ConditionHash, second.ConditionHash)

	var compiled domain.CompiledTriggerCondition
	require.NoError(t, json.Unmarshal(first.CompiledCondition, &compiled))
	assert.Contains(t, compiled.Query, "SELECT COALESCE")
	assert.Contains(t, compiled.Query, "$2", "contact email follows condition arguments")
	assert.Equal(t, []any{"fr"}, compiled.Arguments)
	assert.Equal(t, "root", compiled.RootNodeID)
}

func TestCompilerRejectsInvalidUpdatedField(t *testing.T) {
	automation := ruleAutomation("contact.updated")
	automation.Trigger.UpdatedFields = []string{"email); DROP TABLE contacts; --"}

	_, err := NewTriggerBindingCompiler(NewQueryBuilder()).Compile(automation)
	require.ErrorContains(t, err, "invalid updated_field")
}

func ruleAutomation(eventKind string) *domain.Automation {
	return &domain.Automation{
		ID:         "automation-1",
		RootNodeID: "root",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: eventKind,
			Frequency: domain.TriggerFrequencyEveryTime,
		},
	}
}
