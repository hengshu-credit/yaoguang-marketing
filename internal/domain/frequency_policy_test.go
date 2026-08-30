package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func validFrequencyPolicy() FrequencyPolicy {
	return FrequencyPolicy{ID: "policy-1", Version: 1, Name: "全局邮件频控", Scope: FrequencyScopeWorkspaceGlobal, Channel: "email", MaxEvents: 3, WindowKind: FrequencyWindowSliding, WindowSeconds: 86400, DenyAction: FrequencyActionSuppress, Enabled: true}
}

func TestFrequencyPolicyValidatesIndependentScopes(t *testing.T) {
	global := validFrequencyPolicy()
	assert.NoError(t, global.Validate())
	campaign := global
	campaign.Scope, campaign.ScopeRef = FrequencyScopeCampaign, "campaign-1"
	assert.NoError(t, campaign.Validate())
	trigger := global
	trigger.Scope, trigger.ScopeRef = FrequencyScopeTrigger, "automation-1:event"
	assert.NoError(t, trigger.Validate())
}

func TestFrequencyPolicyRejectsMissingWindowAndCalendarTimezone(t *testing.T) {
	policy := validFrequencyPolicy()
	policy.WindowSeconds = 0
	assert.Error(t, policy.Validate())
	policy = validFrequencyPolicy()
	policy.WindowKind, policy.Timezone = FrequencyWindowCalendar, ""
	assert.ErrorContains(t, policy.Validate(), "timezone")
}
