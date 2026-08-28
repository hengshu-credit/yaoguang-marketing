package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestMailgunService_RegisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://api.notifuse.com/webhooks"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	workspaceID := "workspace123"
	integrationID := "integration456"
	baseURL := "https://api.notifuse.com"
	eventTypes := []domain.EmailEventType{
		domain.EmailEventDelivered,
		domain.EmailEventBounce,
		domain.EmailEventComplaint,
	}

	t.Run("successful registration", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Mock list webhooks response
		listResponse := `{
			"webhooks": {
				"delivered": {
					"urls": []
				},
				"permanent_fail": {
					"urls": []
				},
				"temporary_fail": {
					"urls": []
				},
				"complained": {
					"urls": []
				}
			}
		}`

		// Mock create webhook responses for each event type
		createResponse := `{
			"message": "Webhook has been created",
			"webhook": {
				"url": "https://api.notifuse.com/webhooks/email?provider=mailgun&workspace_id=workspace123&integration_id=integration456"
			}
		}`

		// Setup mock HTTP client for listing webhooks
		listResp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(listResponse)),
		}

		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Contains(t, req.URL.String(), "/webhooks")
				return listResp, nil
			})

		// Track which webhooks were created
		createdWebhooks := map[string]bool{}

		// Setup mock HTTP client for creating webhooks (4 events = 4 API calls)
		// We need to be more flexible here since the order of webhook creation isn't guaranteed
		for i := 0; i < 4; i++ {
			createResp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(createResponse)),
			}

			mockHTTPClient.EXPECT().
				Do(gomock.Any()).
				DoAndReturn(func(req *http.Request) (*http.Response, error) {
					assert.Equal(t, "POST", req.Method)
					assert.Contains(t, req.URL.String(), "/webhooks")

					// Verify form data contains event type and URL
					body, _ := io.ReadAll(req.Body)
					bodyStr := string(body)

					// Check for one of the expected event types
					if strings.Contains(bodyStr, "id=delivered") {
						createdWebhooks["delivered"] = true
					} else if strings.Contains(bodyStr, "id=permanent_fail") {
						createdWebhooks["permanent_fail"] = true
					} else if strings.Contains(bodyStr, "id=temporary_fail") {
						createdWebhooks["temporary_fail"] = true
					} else if strings.Contains(bodyStr, "id=complained") {
						createdWebhooks["complained"] = true
					} else {
						t.Errorf("Unexpected webhook ID in body: %s", bodyStr)
					}

					// Just verify the URL is present
					assert.Contains(t, bodyStr, "url=")

					return createResp, nil
				})
		}

		// Call the service
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailgun, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.Len(t, status.Endpoints, 4) // delivered, permanent_fail, temporary_fail, complained

		// Check provider details
		assert.Equal(t, workspaceID, status.ProviderDetails["workspace_id"])
		assert.Equal(t, integrationID, status.ProviderDetails["integration_id"])

		// Verify all webhooks were created
		assert.True(t, createdWebhooks["delivered"], "delivered webhook should be created")
		assert.True(t, createdWebhooks["permanent_fail"], "permanent_fail webhook should be created")
		assert.True(t, createdWebhooks["temporary_fail"], "temporary_fail webhook should be created")
		assert.True(t, createdWebhooks["complained"], "complained webhook should be created")
	})

	t.Run("missing configuration", func(t *testing.T) {
		// Create invalid email provider config
		invalidConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				// Missing required fields
				Domain: "",
				APIKey: "",
			},
		}

		// Call the service
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, invalidConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("nil configuration", func(t *testing.T) {
		// Call the service with nil config
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, nil)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("list webhooks error", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Setup mock HTTP client to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, assert.AnError)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		status, err := service.RegisterWebhooks(ctx, workspaceID, integrationID, baseURL, eventTypes, providerConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "failed to list Mailgun webhooks")
	})
}

func TestMailgunService_GetWebhookStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://api.notifuse.com/webhooks"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	workspaceID := "workspace123"
	integrationID := "integration456"

	t.Run("successful status check with webhooks", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Mock list webhooks response with registered webhooks
		webhookURL := "https://api.notifuse.com/webhooks/email?provider=mailgun&workspace_id=workspace123&integration_id=integration456"
		listResponse := `{
			"webhooks": {
				"delivered": {
					"urls": ["` + webhookURL + `"]
				},
				"permanent_fail": {
					"urls": ["` + webhookURL + `"]
				},
				"temporary_fail": {
					"urls": []
				},
				"complained": {
					"urls": ["` + webhookURL + `"]
				}
			}
		}`

		// Mock both GETs: the webhooks list and the new routes list (no inbound route).
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				body := listResponse
				if strings.Contains(req.URL.Path, "/routes") {
					body = `{"items":[],"total_count":0}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			}).Times(2)

		// Call the service
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailgun, status.EmailProviderKind)
		assert.True(t, status.IsRegistered)
		assert.Len(t, status.Endpoints, 3) // delivered, permanent_fail, complained

		// Check provider details
		assert.Equal(t, workspaceID, status.ProviderDetails["workspace_id"])
		assert.Equal(t, integrationID, status.ProviderDetails["integration_id"])
		assert.Equal(t, false, status.ProviderDetails["inbound_registered"], "no inbound route present")
	})

	t.Run("inbound route registered is reported", func(t *testing.T) {
		providerConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{Domain: "example.com", APIKey: "test-api-key", Region: "US"},
		}
		emptyWebhooks := `{"webhooks":{"delivered":{"urls":[]},"permanent_fail":{"urls":[]},"temporary_fail":{"urls":[]},"complained":{"urls":[]}}}`
		// A route forwarding inbound mail to this integration's endpoint.
		fwd := fmt.Sprintf(`forward(\"https://api.notifuse.com/webhooks/email/inbound?workspace_id=%s&integration_id=%s\")`, workspaceID, integrationID)
		routes := fmt.Sprintf(`{"total_count":1,"items":[{"id":"r1","actions":["%s","stop()"]}]}`, fwd)

		mockHTTPClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			body := emptyWebhooks
			if strings.Contains(req.URL.Path, "/routes") {
				body = routes
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		}).Times(2)

		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)
		require.NoError(t, err)
		assert.Equal(t, true, status.ProviderDetails["inbound_registered"], "inbound route must be detected")
	})

	t.Run("successful status check with no webhooks", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Mock list webhooks response with no registered webhooks
		listResponse := `{
			"webhooks": {
				"delivered": {
					"urls": []
				},
				"permanent_fail": {
					"urls": []
				},
				"temporary_fail": {
					"urls": []
				},
				"complained": {
					"urls": []
				}
			}
		}`

		// Mock both GETs: the webhooks list and the new routes list (no inbound route).
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				body := listResponse
				if strings.Contains(req.URL.Path, "/routes") {
					body = `{"items":[],"total_count":0}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			}).Times(2)

		// Call the service
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, domain.EmailProviderKindMailgun, status.EmailProviderKind)
		assert.False(t, status.IsRegistered)
		assert.Empty(t, status.Endpoints)
	})

	t.Run("missing configuration", func(t *testing.T) {
		// Create invalid email provider config
		invalidConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				// Missing required fields
				Domain: "",
				APIKey: "",
			},
		}

		// Call the service
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, invalidConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("nil configuration", func(t *testing.T) {
		// Call the service with nil config
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, nil)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("list webhooks error", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Setup mock HTTP client to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, assert.AnError)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		status, err := service.GetWebhookStatus(ctx, workspaceID, integrationID, providerConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "failed to list Mailgun webhooks")
	})
}

func TestMailgunService_UnregisterWebhooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://api.notifuse.com/webhooks"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	workspaceID := "workspace123"
	integrationID := "integration456"

	t.Run("successful unregistration", func(t *testing.T) {
		// Own controller so the path-aware AnyTimes handler doesn't bleed into sibling
		// subtests that share the outer controller.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, httpClient := newMailgunTestService(ctrl)

		providerConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{Domain: "example.com", APIKey: "test-api-key", Region: "US"},
		}

		webhookURL := "https://api.notifuse.com/webhooks/email?provider=mailgun&workspace_id=workspace123&integration_id=integration456"
		listResponse := `{"webhooks":{"delivered":{"urls":["` + webhookURL + `"]},"permanent_fail":{"urls":["` + webhookURL + `"]},"temporary_fail":{"urls":[]},"complained":{"urls":["` + webhookURL + `"]}}}`
		// A routes list containing OUR inbound reply route, so unregister must delete it.
		fwd := `forward(\"https://api.notifuse.com/webhooks/email/inbound?workspace_id=workspace123&integration_id=integration456\")`
		routesBody := fmt.Sprintf(`{"total_count":1,"items":[{"id":"route-1","actions":["%s","stop()"]}]}`, fwd)

		deletedEvents := map[string]bool{}
		deletedRoute := false
		httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			ok := func(b string) *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(b))}
			}
			switch {
			case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/routes"):
				return ok(routesBody), nil
			case req.Method == http.MethodGet:
				return ok(listResponse), nil
			case req.Method == http.MethodDelete && strings.Contains(req.URL.Path, "/routes/"):
				deletedRoute = true
				return ok(`{"message":"Route has been deleted"}`), nil
			case req.Method == http.MethodDelete:
				u := req.URL.String()
				switch {
				case strings.Contains(u, "/webhooks/delivered"):
					deletedEvents["delivered"] = true
				case strings.Contains(u, "/webhooks/permanent_fail"):
					deletedEvents["permanent_fail"] = true
				case strings.Contains(u, "/webhooks/complained"):
					deletedEvents["complained"] = true
				default:
					t.Errorf("unexpected DELETE URL: %s", u)
				}
				return ok(`{"message":"Webhook has been deleted"}`), nil
			default:
				return ok(``), nil
			}
		}).AnyTimes()

		err := svc.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)
		require.NoError(t, err)

		assert.True(t, deletedEvents["delivered"])
		assert.True(t, deletedEvents["permanent_fail"])
		assert.True(t, deletedEvents["complained"])
		assert.True(t, deletedRoute, "the inbound reply route must be deleted on unregister")
	})

	t.Run("missing configuration", func(t *testing.T) {
		// Create invalid email provider config
		invalidConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				// Missing required fields
				Domain: "",
				APIKey: "",
			},
		}

		// Call the service
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, invalidConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("nil configuration", func(t *testing.T) {
		// Call the service with nil config
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, nil)

		// Verify error is returned
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration is missing or invalid")
	})

	t.Run("list webhooks error", func(t *testing.T) {
		// Create email provider config
		providerConfig := &domain.EmailProvider{
			Kind: domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{
				Domain: "example.com",
				APIKey: "test-api-key",
				Region: "US",
			},
		}

		// Setup mock HTTP client to return error
		mockHTTPClient.EXPECT().
			Do(gomock.Any()).
			Return(nil, assert.AnError)

		// Allow any logger calls since we don't test logging
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the service
		err := service.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)

		// Verify error is returned
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Mailgun webhooks")
	})

	t.Run("delete webhook error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, httpClient := newMailgunTestService(ctrl)

		providerConfig := &domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailgun,
			Mailgun: &domain.MailgunSettings{Domain: "example.com", APIKey: "test-api-key", Region: "US"},
		}
		webhookURL := "https://api.notifuse.com/webhooks/email?provider=mailgun&workspace_id=workspace123&integration_id=integration456"
		listResponse := `{"webhooks":{"delivered":{"urls":["` + webhookURL + `"]},"permanent_fail":{"urls":[]},"temporary_fail":{"urls":[]},"complained":{"urls":[]}}}`

		httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			ok := func(b string) *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(b))}
			}
			switch {
			case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/routes"):
				return ok(`{"total_count":0,"items":[]}`), nil
			case req.Method == http.MethodGet:
				return ok(listResponse), nil
			case req.Method == http.MethodDelete:
				return nil, assert.AnError // webhook delete fails
			default:
				return ok(``), nil
			}
		}).AnyTimes()

		err := svc.UnregisterWebhooks(ctx, workspaceID, integrationID, providerConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unregister one or more Mailgun webhooks")
	})
}

func TestMapMailgunEventType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected domain.EmailEventType
	}{
		{
			name:     "delivered event",
			input:    "delivered",
			expected: domain.EmailEventDelivered,
		},
		{
			name:     "permanent fail event",
			input:    "permanent_fail",
			expected: domain.EmailEventBounce,
		},
		{
			name:     "temporary fail event",
			input:    "temporary_fail",
			expected: domain.EmailEventBounce,
		},
		{
			name:     "complained event",
			input:    "complained",
			expected: domain.EmailEventComplaint,
		},
		{
			name:     "unknown event",
			input:    "unknown_event",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapMailgunEventType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMailgunService_TestWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := mocks.NewMockHTTPClient(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	webhookEndpoint := "https://api.notifuse.com/webhooks"
	service := NewMailgunService(mockHTTPClient, mockAuthService, mockLogger, webhookEndpoint)

	ctx := context.Background()
	config := domain.MailgunSettings{
		Domain: "example.com",
		APIKey: "test-api-key",
		Region: "US",
	}
	webhookID := "delivered"
	eventType := "delivered"

	// Call the service
	err := service.TestWebhook(ctx, config, webhookID, eventType)

	// Verify error is returned with the expected message
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "testing webhooks is not supported by the Mailgun API")
}
