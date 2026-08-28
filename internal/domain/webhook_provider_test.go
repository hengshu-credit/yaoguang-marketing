package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestGenerateWebhookCallbackURL(t *testing.T) {
	testCases := []struct {
		name          string
		baseURL       string
		provider      domain.EmailProviderKind
		workspaceID   string
		integrationID string
		expected      string
	}{
		{
			name:          "Standard URL",
			baseURL:       "https://api.example.com",
			provider:      domain.EmailProviderKindPostmark,
			workspaceID:   "workspace123",
			integrationID: "integration123",
			expected:      "https://api.example.com/webhooks/email?provider=postmark&workspace_id=workspace123&integration_id=integration123",
		},
		{
			name:          "Base URL with trailing slash",
			baseURL:       "https://api.example.com/",
			provider:      domain.EmailProviderKindMailgun,
			workspaceID:   "ws-456",
			integrationID: "int-789",
			expected:      "https://api.example.com//webhooks/email?provider=mailgun&workspace_id=ws-456&integration_id=int-789",
		},
		{
			name:          "Empty workspace ID",
			baseURL:       "https://api.example.com",
			provider:      domain.EmailProviderKindSES,
			workspaceID:   "",
			integrationID: "integration123",
			expected:      "https://api.example.com/webhooks/email?provider=ses&workspace_id=&integration_id=integration123",
		},
		{
			name:          "Special characters",
			baseURL:       "https://api.example.com",
			provider:      domain.EmailProviderKindSparkPost,
			workspaceID:   "workspace 123!",
			integrationID: "integration&123",
			expected:      "https://api.example.com/webhooks/email?provider=sparkpost&workspace_id=workspace 123!&integration_id=integration&123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := domain.GenerateWebhookCallbackURL(tc.baseURL, tc.provider, tc.workspaceID, tc.integrationID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateInboundWebhookURL(t *testing.T) {
	result := domain.GenerateInboundWebhookURL("https://api.example.com", "ws-123", "int-456")
	assert.Equal(t, "https://api.example.com/webhooks/email/inbound?workspace_id=ws-123&integration_id=int-456", result)
}

func TestWebhookRegistrationStatus_JSON(t *testing.T) {
	// Create a test status
	status := &domain.WebhookRegistrationStatus{
		EmailProviderKind: domain.EmailProviderKindPostmark,
		IsRegistered:      true,
		Endpoints: []domain.WebhookEndpointStatus{
			{
				WebhookID: "webhook123",
				URL:       "https://api.example.com/webhooks/email",
				EventType: domain.EmailEventDelivered,
				Active:    true,
			},
			{
				WebhookID: "webhook456",
				URL:       "https://api.example.com/webhooks/email",
				EventType: domain.EmailEventBounce,
				Active:    false,
			},
		},
		ProviderDetails: map[string]interface{}{
			"server_id": "12345",
			"account":   "test-account",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(status)
	require.NoError(t, err)

	// Unmarshal back to verify
	var decodedStatus domain.WebhookRegistrationStatus
	err = json.Unmarshal(jsonData, &decodedStatus)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, domain.EmailProviderKindPostmark, decodedStatus.EmailProviderKind)
	assert.True(t, decodedStatus.IsRegistered)
	assert.Len(t, decodedStatus.Endpoints, 2)

	assert.Equal(t, "webhook123", decodedStatus.Endpoints[0].WebhookID)
	assert.Equal(t, "https://api.example.com/webhooks/email", decodedStatus.Endpoints[0].URL)
	assert.Equal(t, domain.EmailEventDelivered, decodedStatus.Endpoints[0].EventType)
	assert.True(t, decodedStatus.Endpoints[0].Active)

	assert.Equal(t, "webhook456", decodedStatus.Endpoints[1].WebhookID)
	assert.Equal(t, domain.EmailEventBounce, decodedStatus.Endpoints[1].EventType)
	assert.False(t, decodedStatus.Endpoints[1].Active)

	assert.Equal(t, "12345", decodedStatus.ProviderDetails["server_id"])
	assert.Equal(t, "test-account", decodedStatus.ProviderDetails["account"])
}

func TestWebhookRegistrationRequest_Validate(t *testing.T) {
	t.Run("Valid request", func(t *testing.T) {
		req := &domain.RegisterWebhookRequest{
			WorkspaceID:   "workspace123",
			IntegrationID: "integration123",
			EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered, domain.EmailEventBounce},
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Len(t, req.EventTypes, 2)
	})

	t.Run("Missing workspace ID", func(t *testing.T) {
		req := &domain.RegisterWebhookRequest{
			IntegrationID: "integration123",
			EventTypes:    []domain.EmailEventType{domain.EmailEventDelivered},
		}

		err := req.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("Missing integration ID", func(t *testing.T) {
		req := &domain.RegisterWebhookRequest{
			WorkspaceID: "workspace123",
			EventTypes:  []domain.EmailEventType{domain.EmailEventDelivered},
		}

		err := req.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "integration_id is required")
	})

	t.Run("No event types - defaults to all", func(t *testing.T) {
		req := &domain.RegisterWebhookRequest{
			WorkspaceID:   "workspace123",
			IntegrationID: "integration123",
		}

		err := req.Validate()
		assert.NoError(t, err)

		// Should default to all three event types
		assert.Len(t, req.EventTypes, 3)
		assert.Contains(t, req.EventTypes, domain.EmailEventDelivered)
		assert.Contains(t, req.EventTypes, domain.EmailEventBounce)
		assert.Contains(t, req.EventTypes, domain.EmailEventComplaint)
	})
}

func TestGetWebhookStatusRequest_Validate(t *testing.T) {
	t.Run("Valid request", func(t *testing.T) {
		req := &domain.GetWebhookStatusRequest{
			WorkspaceID:   "workspace123",
			IntegrationID: "integration123",
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("Missing workspace ID", func(t *testing.T) {
		req := &domain.GetWebhookStatusRequest{
			IntegrationID: "integration123",
		}

		err := req.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("Missing integration ID", func(t *testing.T) {
		req := &domain.GetWebhookStatusRequest{
			WorkspaceID: "workspace123",
		}

		err := req.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "integration_id is required")
	})

	t.Run("JSON serialization", func(t *testing.T) {
		req := &domain.GetWebhookStatusRequest{
			WorkspaceID:   "workspace123",
			IntegrationID: "integration456",
		}

		jsonData, err := json.Marshal(req)
		require.NoError(t, err)

		var decodedReq domain.GetWebhookStatusRequest
		err = json.Unmarshal(jsonData, &decodedReq)
		require.NoError(t, err)

		assert.Equal(t, "workspace123", decodedReq.WorkspaceID)
		assert.Equal(t, "integration456", decodedReq.IntegrationID)
	})
}
