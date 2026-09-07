package service

import (
	"encoding/json"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestConfiguredTriggerCompilesArbitraryObjectEvents(t *testing.T) {
	a := &domain.Automation{ID: "flow", RootNodeID: "start", Trigger: &domain.TimelineTriggerConfig{EventKind: "tracked_event", Frequency: domain.TriggerFrequencyEveryTime, EventRule: &domain.MarketingEventRule{SubjectType: "contract", Events: []string{"contract.signed", "contract.renewed"}, OncePerObject: false}}}
	bindings, err := NewTriggerBindingCompiler(NewQueryBuilder()).CompileAll(a)
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "contract", bindings[0].SubjectType)
	require.Equal(t, "contract.signed", bindings[0].EventType)
	require.Equal(t, "contract.renewed", bindings[1].EventType)
	require.Equal(t, bindings[0].ConditionHash, bindings[1].ConditionHash)
	var condition domain.CompiledTriggerCondition
	require.NoError(t, json.Unmarshal(bindings[0].CompiledCondition, &condition))
	require.Equal(t, "SELECT TRUE", condition.Query)
	require.Equal(t, domain.TriggerFrequencyEveryTime, condition.Frequency)
}

func TestConfiguredTriggerRejectsMissingSubjectAndEmailCondition(t *testing.T) {
	trigger := domain.TimelineTriggerConfig{EventKind: "tracked_event", Frequency: domain.TriggerFrequencyEveryTime, EventRule: &domain.MarketingEventRule{Events: []string{"signed"}}}
	require.Error(t, trigger.Validate())
	trigger.EventRule.SubjectType = "contract"
	require.NoError(t, trigger.Validate())
	trigger.Conditions = &domain.TreeNode{}
	require.ErrorContains(t, trigger.Validate(), "event rule")
}
