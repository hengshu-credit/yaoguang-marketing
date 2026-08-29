package app

import (
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/stretchr/testify/assert"
)

func TestAppRuntimeRoleControlsCapabilities(t *testing.T) {
	application := &App{config: &config.Config{
		Realtime: config.RealtimeConfig{Role: config.RoleAPI},
	}}

	assert.True(t, application.runs(config.CapabilityHTTP))
	assert.False(t, application.runs(config.CapabilityOutboxRelay))
	assert.False(t, application.runs(config.CapabilityJourney))
}
