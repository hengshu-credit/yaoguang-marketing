package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChannelMessageService struct {
	request  *domain.SendChannelMessageRequest
	response *domain.SendChannelMessageResponse
	err      error
}

func (s *fakeChannelMessageService) Send(_ context.Context, request *domain.SendChannelMessageRequest) (*domain.SendChannelMessageResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestChannelMessageHandlerSend(t *testing.T) {
	service := &fakeChannelMessageService{response: &domain.SendChannelMessageResponse{Execution: domain.ChannelSendExecution{
		EffectKey: "effect-1", Status: domain.ChannelSendConfirmed, ProviderMessageID: "SM123",
	}}}
	handler := NewChannelMessageHandler(service, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/channelMessages.send", strings.NewReader(`{
		"workspace_id":"ws-1","effect_key":"effect-1","channel":"sms",
		"integration_id":"twilio-main","contact_email":"user@example.com","template_id":"ready"
	}`))
	request.Header.Set("Content-Type", "application/json")
	handler.handleSend(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, service.request)
	assert.Equal(t, "effect-1", service.request.EffectKey)
	var response domain.SendChannelMessageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, domain.ChannelSendConfirmed, response.Execution.Status)
}

func TestChannelMessageHandlerRejectsOversizedBody(t *testing.T) {
	service := &fakeChannelMessageService{}
	handler := NewChannelMessageHandler(service, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/channelMessages.send", strings.NewReader(`{"workspace_id":"`+strings.Repeat("x", int(channelMessageMaxBodyBytes))+`"}`))
	handler.handleSend(recorder, request)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestChannelMessageHandlerRejectsTrailingJSON(t *testing.T) {
	service := &fakeChannelMessageService{}
	handler := NewChannelMessageHandler(service, func() ([]byte, error) { return []byte("secret"), nil }, logger.NewLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/channelMessages.send", strings.NewReader(`{
		"workspace_id":"ws-1","effect_key":"effect-1","channel":"sms",
		"integration_id":"twilio-main","contact_email":"user@example.com","template_id":"ready"
	} {}`))
	handler.handleSend(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Nil(t, service.request)
}
