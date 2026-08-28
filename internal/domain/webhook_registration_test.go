package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterWebhookRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request RegisterWebhookRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid request with all fields",
			request: RegisterWebhookRequest{
				WorkspaceID:   "ws-123",
				IntegrationID: "int-123",
				EventTypes: []EmailEventType{
					EmailEventDelivered,
					EmailEventBounce,
				},
			},
			wantErr: false,
		},
		{
			name: "Valid request with no event types (should default to all)",
			request: RegisterWebhookRequest{
				WorkspaceID:   "ws-123",
				IntegrationID: "int-123",
			},
			wantErr: false,
		},
		{
			name: "Missing workspace ID",
			request: RegisterWebhookRequest{
				IntegrationID: "int-123",
				EventTypes:    []EmailEventType{EmailEventDelivered},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "Missing integration ID",
			request: RegisterWebhookRequest{
				WorkspaceID: "ws-123",
				EventTypes:  []EmailEventType{EmailEventDelivered},
			},
			wantErr: true,
			errMsg:  "integration_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				// If no event types were specified, check that defaults were added
				if len(tt.request.EventTypes) == 0 {
					assert.Equal(t, 3, len(tt.request.EventTypes))
					assert.Contains(t, tt.request.EventTypes, EmailEventDelivered)
					assert.Contains(t, tt.request.EventTypes, EmailEventBounce)
					assert.Contains(t, tt.request.EventTypes, EmailEventComplaint)
				}
			}
		})
	}
}

func TestGetWebhookStatusRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request GetWebhookStatusRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid request",
			request: GetWebhookStatusRequest{
				WorkspaceID:   "ws-123",
				IntegrationID: "int-123",
			},
			wantErr: false,
		},
		{
			name: "Missing workspace ID",
			request: GetWebhookStatusRequest{
				IntegrationID: "int-123",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "Missing integration ID",
			request: GetWebhookStatusRequest{
				WorkspaceID: "ws-123",
			},
			wantErr: true,
			errMsg:  "integration_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWebhookRegistrationStatus(t *testing.T) {
	status := &WebhookRegistrationStatus{
		EmailProviderKind: EmailProviderKindSES,
		IsRegistered:      true,
		Endpoints: []WebhookEndpointStatus{
			{
				URL:       "https://example.com/webhooks/ses/delivered",
				EventType: EmailEventDelivered,
				Active:    true,
			},
			{
				URL:       "https://example.com/webhooks/ses/bounce",
				EventType: EmailEventBounce,
				Active:    true,
			},
			{
				URL:       "https://example.com/webhooks/ses/complaint",
				EventType: EmailEventComplaint,
				Active:    true,
			},
		},
		ProviderDetails: map[string]interface{}{
			"configuration_set": "my-configuration-set",
		},
	}

	// Basic structure verification
	assert.Equal(t, EmailProviderKindSES, status.EmailProviderKind)
	assert.True(t, status.IsRegistered)
	assert.Len(t, status.Endpoints, 3)
	assert.Equal(t, "my-configuration-set", status.ProviderDetails["configuration_set"])

	// Check endpoint content
	assert.Equal(t, EmailEventDelivered, status.Endpoints[0].EventType)
	assert.Equal(t, "https://example.com/webhooks/ses/delivered", status.Endpoints[0].URL)
	assert.True(t, status.Endpoints[0].Active)
}

func TestWebhookRegistrationConfig(t *testing.T) {
	config := &WebhookRegistrationConfig{
		IntegrationID: "int-123",
		EventTypes: []EmailEventType{
			EmailEventDelivered,
			EmailEventBounce,
		},
	}

	// Basic structure verification
	assert.Equal(t, "int-123", config.IntegrationID)
	assert.Len(t, config.EventTypes, 2)
	assert.Contains(t, config.EventTypes, EmailEventDelivered)
	assert.Contains(t, config.EventTypes, EmailEventBounce)
}
