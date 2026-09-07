package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSendChannelMessageAllowsCanonicalCustomerWithoutEmail(t *testing.T) {
	r := &SendChannelMessageRequest{WorkspaceID: "ws", EffectKey: "effect", Channel: "sms", IntegrationID: "provider", CustomerID: "11111111-1111-4111-8111-111111111111", TemplateID: "template"}
	require.NoError(t, r.Validate())
	r.CustomerID = "bad"
	require.Error(t, r.Validate())
}
