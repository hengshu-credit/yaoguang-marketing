package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	pkgmocks "github.com/hengshu-credit/yaoguang-marketing/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workerDeliveryRepository struct {
	startErr         error
	attempt          domain.DeliveryAttempt
	outcomes         []domain.DeliveryAttemptOutcome
	outcomeErr       map[domain.DeliveryStatus]error
	suppressedReason string
}

func (r *workerDeliveryRepository) ReserveIntent(context.Context, string, domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	return domain.DeliveryIntent{}, false, nil
}
func (r *workerDeliveryRepository) ReserveAndEnqueue(context.Context, string, domain.DeliveryIntent, *domain.EmailQueueEntry) (domain.ReserveDeliveryResult, error) {
	return domain.ReserveDeliveryResult{}, nil
}
func (r *workerDeliveryRepository) ResolveCustomerID(context.Context, string, string) (string, error) {
	return "", nil
}
func (r *workerDeliveryRepository) GetIntentByEffectKey(context.Context, string, string) (*domain.DeliveryIntent, error) {
	return nil, nil
}
func (r *workerDeliveryRepository) TransitionIntent(context.Context, string, string, domain.DeliveryStatus, domain.DeliveryStatus, time.Time) (bool, error) {
	return true, nil
}
func (r *workerDeliveryRepository) StartAttempt(context.Context, string, domain.DeliveryAttemptStart) (domain.DeliveryAttempt, error) {
	if r.startErr != nil {
		return domain.DeliveryAttempt{}, r.startErr
	}
	return r.attempt, nil
}
func (r *workerDeliveryRepository) RecordAttemptOutcome(_ context.Context, _ string, _ string, _ string, outcome domain.DeliveryAttemptOutcome) error {
	r.outcomes = append(r.outcomes, outcome)
	return r.outcomeErr[outcome.Status]
}

func (r *workerDeliveryRepository) SuppressIntent(_ context.Context, _ string, _ string, _ domain.DeliveryStatus, reason string, _ time.Time) (bool, error) {
	r.suppressedReason = reason
	return true, nil
}

type workerAudienceEligibilityStub struct {
	result bool
	err    error
	calls  int
}

func (s *workerAudienceEligibilityStub) MatchesCustomerInternal(context.Context, string, string, int, string) (bool, error) {
	s.calls++
	return s.result, s.err
}

type deliveryWorkerFixture struct {
	worker    *EmailQueueWorker
	queue     *mocks.MockEmailQueueRepository
	email     *mocks.MockEmailServiceInterface
	history   *mocks.MockMessageHistoryRepository
	workspace *domain.Workspace
	entry     *domain.EmailQueueEntry
	delivery  *workerDeliveryRepository
}

func newDeliveryWorkerFixture(t *testing.T) deliveryWorkerFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	emailService := mocks.NewMockEmailServiceInterface(ctrl)
	historyRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	deliveryRepo := &workerDeliveryRepository{
		attempt: domain.DeliveryAttempt{
			ID:        "44444444-4444-4444-8444-444444444444",
			IntentID:  "11111111-1111-4111-8111-111111111111",
			Status:    domain.DeliveryStatusSubmitting,
			EffectKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		outcomeErr: map[domain.DeliveryStatus]error{},
	}
	worker := NewEmailQueueWorker(queueRepo, workspaceRepo, emailService, historyRepo, DefaultWorkerConfig(), log)
	worker.SetDeliveryRepository(deliveryRepo)
	worker.ctx = context.Background()

	workspace := &domain.Workspace{
		ID: "workspace-1", Settings: domain.WorkspaceSettings{SecretKey: "secret"},
		Integrations: []domain.Integration{{
			ID: "integration-1", EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 60000,
			},
		}},
	}
	lease := time.Now().UTC().Add(time.Minute)
	entry := &domain.EmailQueueEntry{
		ID: "queue-1", DeliveryIntentID: deliveryRepo.attempt.IntentID,
		ClaimToken: "33333333-3333-4333-8333-333333333333", LeaseExpiresAt: &lease,
		Status: domain.EmailQueueStatusProcessing, SourceType: domain.EmailQueueSourceBroadcast,
		SourceID: "broadcast-1", IntegrationID: "integration-1", ProviderKind: domain.EmailProviderKindSMTP,
		ContactEmail: "recipient@example.com", MessageID: "message-1", TemplateID: "template-1",
		Payload: domain.EmailQueuePayload{
			FromAddress: "sender@example.com", FromName: "Sender", Subject: "Hello", HTMLContent: "<p>Hello</p>",
		},
		Attempts: 1, MaxAttempts: 3, CreatedAt: time.Now().UTC(),
	}
	return deliveryWorkerFixture{worker, queueRepo, emailService, historyRepo, workspace, entry, deliveryRepo}
}

func TestDeliveryWorkerDoesNotCallProviderWhenSubmittingPersistenceFails(t *testing.T) {
	fixture := newDeliveryWorkerFixture(t)
	fixture.delivery.startErr = errors.New("database unavailable")

	fixture.worker.processEntry(fixture.workspace, fixture.entry)

	assert.Empty(t, fixture.delivery.outcomes)
}

func TestDeliveryWorkerRechecksAudienceImmediatelyBeforeProviderAndSuppressesPaidCustomer(t *testing.T) {
	fixture := newDeliveryWorkerFixture(t)
	checker := &workerAudienceEligibilityStub{result: false}
	fixture.worker.SetAudienceEligibilityChecker(checker)
	fixture.entry.Payload.AudienceEligibility = &domain.AudienceEligibilityContext{
		AudienceID: "audience-1", AudienceVersion: 7, AudienceBuildID: "build-7", CustomerID: "customer-1",
	}
	fixture.queue.EXPECT().CompleteClaim(gomock.Any(), "workspace-1", fixture.entry.ID, fixture.entry.ClaimToken, gomock.Any()).Return(nil)

	fixture.worker.processEntry(fixture.workspace, fixture.entry)

	assert.Equal(t, 1, checker.calls)
	assert.Equal(t, "audience_no_longer_matched", fixture.delivery.suppressedReason)
	assert.Empty(t, fixture.delivery.outcomes)
}

func TestDeliveryWorkerPersistsAcceptedHistoryAndConfirmed(t *testing.T) {
	fixture := newDeliveryWorkerFixture(t)
	fixture.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).DoAndReturn(
		func(_ context.Context, request domain.SendEmailProviderRequest, _ bool) error {
			assert.Equal(t, fixture.delivery.attempt.EffectKey, request.IdempotencyKey)
			return nil
		})
	fixture.history.EXPECT().Upsert(gomock.Any(), "workspace-1", "secret", gomock.Any()).Return(nil)

	fixture.worker.processEntry(fixture.workspace, fixture.entry)

	require.Len(t, fixture.delivery.outcomes, 2)
	assert.Equal(t, domain.DeliveryStatusProviderAccepted, fixture.delivery.outcomes[0].Status)
	assert.Equal(t, domain.DeliveryStatusConfirmed, fixture.delivery.outcomes[1].Status)
}

func TestDeliveryWorkerLeavesProviderAcceptedWhenHistoryConfirmationFails(t *testing.T) {
	fixture := newDeliveryWorkerFixture(t)
	fixture.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
	fixture.history.EXPECT().Upsert(gomock.Any(), "workspace-1", "secret", gomock.Any()).Return(errors.New("history unavailable"))

	fixture.worker.processEntry(fixture.workspace, fixture.entry)

	require.Len(t, fixture.delivery.outcomes, 1)
	assert.Equal(t, domain.DeliveryStatusProviderAccepted, fixture.delivery.outcomes[0].Status)
}

func TestDeliveryWorkerMarksTransportTimeoutUnknownWithoutRetry(t *testing.T) {
	fixture := newDeliveryWorkerFixture(t)
	fixture.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(context.DeadlineExceeded)
	fixture.history.EXPECT().Upsert(gomock.Any(), "workspace-1", "secret", gomock.Any()).Return(nil)

	fixture.worker.processEntry(fixture.workspace, fixture.entry)

	require.Len(t, fixture.delivery.outcomes, 1)
	assert.Equal(t, domain.DeliveryStatusUnknown, fixture.delivery.outcomes[0].Status)
	assert.Nil(t, fixture.delivery.outcomes[0].NextRetryAt)
}
