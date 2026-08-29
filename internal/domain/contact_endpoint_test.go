package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactEndpointMutationValidateAndNormalize(t *testing.T) {
	mutation := ContactEndpointMutation{
		Operation:  EndpointOperationUpsert,
		EndpointID: " device-1 ",
		Channel:    ChannelPush,
		Provider:   PushProviderFCM,
		Platform:   EndpointPlatformAndroid,
		Address:    " token-value ",
		Locale:     " zh-CN ",
		Timezone:   " Asia/Shanghai ",
	}

	endpoint, err := mutation.Validate()
	require.NoError(t, err)
	assert.Equal(t, "device-1", endpoint.EndpointID)
	assert.Equal(t, "token-value", endpoint.Address)
	assert.Equal(t, "zh-CN", endpoint.Locale)
	assert.Equal(t, "Asia/Shanghai", endpoint.Timezone)
	assert.True(t, endpoint.Enabled)

	encoded, err := json.Marshal(endpoint)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "token-value")
	assert.NotContains(t, string(encoded), "address")
}

func TestContactEndpointMutationRejectsInvalidProviderPlatformCombination(t *testing.T) {
	_, err := (ContactEndpointMutation{
		Operation:  EndpointOperationUpsert,
		EndpointID: "device-1",
		Channel:    ChannelPush,
		Provider:   PushProviderAPNS,
		Platform:   EndpointPlatformAndroid,
		Address:    "token",
	}).Validate()
	assert.EqualError(t, err, "provider apns requires platform ios")
}

func TestContactEndpointDisableRequiresOnlyStableID(t *testing.T) {
	endpoint, err := (ContactEndpointMutation{
		Operation:  EndpointOperationDisable,
		EndpointID: "device-1",
	}).Validate()
	require.NoError(t, err)
	assert.Equal(t, "device-1", endpoint.EndpointID)
	assert.False(t, endpoint.Enabled)
}

func TestContactEndpointMutationAcceptsTwilioSMSPhone(t *testing.T) {
	endpoint, err := (ContactEndpointMutation{
		Operation:  EndpointOperationUpsert,
		EndpointID: "phone-primary",
		Channel:    ChannelSMS,
		Provider:   EndpointProviderTwilio,
		Platform:   EndpointPlatformPhone,
		Address:    " +15557654321 ",
		Locale:     "en-US",
	}).Validate()
	require.NoError(t, err)
	assert.Equal(t, "+15557654321", endpoint.Address)
	assert.Equal(t, EndpointProviderTwilio, endpoint.Provider)
	assert.Equal(t, EndpointPlatformPhone, endpoint.Platform)
}

func TestContactEndpointMutationRejectsInvalidSMSPhone(t *testing.T) {
	_, err := (ContactEndpointMutation{
		Operation: EndpointOperationUpsert, EndpointID: "phone-primary",
		Channel: ChannelSMS, Provider: EndpointProviderTwilio,
		Platform: EndpointPlatformPhone, Address: "555-765-4321",
	}).Validate()
	assert.EqualError(t, err, "twilio sms address must be in E.164 format")
}
