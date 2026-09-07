package queue

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	pkgmocks "github.com/hengshu-credit/yaoguang-marketing/pkg/mocks"
	"github.com/stretchr/testify/require"
	"testing"
)

type executionProviderResolverStub struct {
	integration *domain.Integration
	executionID string
}

func (s *executionProviderResolverStub) ResolveExecutionIntegration(_ context.Context, _, executionID, _ string, _ *domain.Workspace) (*domain.Integration, error) {
	s.executionID = executionID
	return s.integration, nil
}

func TestEmailQueueWorkerExecutionUsesCapturedProviderAtActualSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	queue := mocks.NewMockEmailQueueRepository(ctrl)
	workspaces := mocks.NewMockWorkspaceRepository(ctrl)
	emails := mocks.NewMockEmailServiceInterface(ctrl)
	history := mocks.NewMockMessageHistoryRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	current := &domain.Workspace{ID: "ws", Integrations: domain.Integrations{{ID: "smtp", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 100, SMTP: &domain.SMTPSettings{Host: "new.example.org", Password: "new-password"}}}}}
	original := &domain.Integration{ID: "smtp", EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 100, SMTP: &domain.SMTPSettings{Host: "old.example.org", Password: "old-password"}}}
	resolver := &executionProviderResolverStub{integration: original}
	entry := &domain.EmailQueueEntry{ID: "entry", SourceType: domain.EmailQueueSourceBroadcast, IntegrationID: "smtp", ContactEmail: "customer@example.org", MessageID: "message", MaxAttempts: 3, Payload: domain.EmailQueuePayload{ExecutionID: "old-run", FromAddress: "sender@example.org", Subject: "Saved subject", HTMLContent: "<p>Saved content</p>"}}
	queue.EXPECT().MarkAsProcessing(gomock.Any(), "ws", "entry").Return(nil)
	emails.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).DoAndReturn(func(_ context.Context, request domain.SendEmailProviderRequest, _ bool) error {
		require.Equal(t, "old.example.org", request.Provider.SMTP.Host)
		require.Equal(t, "old-password", request.Provider.SMTP.Password)
		require.Equal(t, "<p>Saved content</p>", request.Content)
		return nil
	})
	history.EXPECT().Upsert(gomock.Any(), "ws", gomock.Any(), gomock.Any()).Return(nil)
	queue.EXPECT().MarkAsSent(gomock.Any(), "ws", "entry").Return(nil)
	worker := NewEmailQueueWorker(queue, workspaces, emails, history, DefaultWorkerConfig(), logger)
	worker.ctx = context.Background()
	worker.SetExecutionProviderResolver(resolver)
	worker.processEntry(current, entry)
	require.Equal(t, "old-run", resolver.executionID)
	require.Equal(t, "new-password", current.Integrations[0].EmailProvider.SMTP.Password)
}
