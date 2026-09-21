package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/require"
)

func TestTransactionalSendDistinguishesConfigurationFromTransientFailure(t *testing.T) {
	for _, tc := range []struct {
		reason string
		status int
	}{
		{"no email provider configured", http.StatusBadRequest},
		{"failed to send notification through any channel", http.StatusInternalServerError},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			service := mocks.NewMockTransactionalNotificationService(ctrl)
			service.EXPECT().SendNotification(gomock.Any(), "ws", gomock.Any()).Return("", errors.New(tc.reason))
			handler := NewTransactionalNotificationHandler(service, nil, logger.NewLogger(), false)
			request := httptest.NewRequest(http.MethodPost, "/api/transactional.send", strings.NewReader(`{"workspace_id":"ws","notification":{"id":"welcome","contact":{"email":"test@example.com"},"channels":["email"]}}`))
			response := httptest.NewRecorder()
			handler.handleSend(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
		})
	}
}
