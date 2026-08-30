package broadcast

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

type capturedDeliveryReservation struct {
	intent domain.DeliveryIntent
	entry  domain.EmailQueueEntry
}

type captureBroadcastDeliveryRepository struct {
	reservations []capturedDeliveryReservation
	failAt       int
	missingEmail string
}

func (r *captureBroadcastDeliveryRepository) ReserveIntent(context.Context, string, domain.DeliveryIntent) (domain.DeliveryIntent, bool, error) {
	return domain.DeliveryIntent{}, false, nil
}

func (r *captureBroadcastDeliveryRepository) ReserveAndEnqueue(_ context.Context, _ string, intent domain.DeliveryIntent, entry *domain.EmailQueueEntry) (domain.ReserveDeliveryResult, error) {
	r.reservations = append(r.reservations, capturedDeliveryReservation{intent: intent, entry: *entry})
	if r.failAt > 0 && len(r.reservations) == r.failAt {
		return domain.ReserveDeliveryResult{}, errors.New("injected reservation failure")
	}
	intent.Status = domain.DeliveryStatusQueued
	return domain.ReserveDeliveryResult{Intent: intent, Created: true, QueueCreated: true}, nil
}

func (r *captureBroadcastDeliveryRepository) ResolveCustomerID(_ context.Context, _ string, email string) (string, error) {
	if email == r.missingEmail {
		return "", nil
	}
	if email == "one@example.com" {
		return "11111111-1111-4111-8111-111111111111", nil
	}
	return "22222222-2222-4222-8222-222222222222", nil
}

func TestQueueMessageSenderDeliveryRequiresAuthorityCustomerID(t *testing.T) {
	deliveryRepo := &captureBroadcastDeliveryRepository{missingEmail: "one@example.com"}
	sender, recipients, templates, provider := deliveryQueueSenderFixture(t, deliveryRepo)

	sent, failed, err := sender.SendBatch(
		context.Background(), "workspace-1", "integration-1", "secret",
		"https://api.example.com", "", false, nil, "broadcast-1", recipients,
		templates, provider, time.Now().Add(time.Minute), "",
	)
	assert.ErrorContains(t, err, "delivery customer authority is missing")
	assert.Zero(t, sent)
	assert.Zero(t, failed)
	assert.Empty(t, deliveryRepo.reservations)
}

func (r *captureBroadcastDeliveryRepository) GetIntentByEffectKey(context.Context, string, string) (*domain.DeliveryIntent, error) {
	return nil, nil
}

func (r *captureBroadcastDeliveryRepository) TransitionIntent(context.Context, string, string, domain.DeliveryStatus, domain.DeliveryStatus, time.Time) (bool, error) {
	return true, nil
}

func (r *captureBroadcastDeliveryRepository) StartAttempt(context.Context, string, domain.DeliveryAttemptStart) (domain.DeliveryAttempt, error) {
	return domain.DeliveryAttempt{}, nil
}

func (r *captureBroadcastDeliveryRepository) RecordAttemptOutcome(context.Context, string, string, string, domain.DeliveryAttemptOutcome) error {
	return nil
}

func deliveryQueueSenderFixture(t *testing.T, deliveryRepo domain.DeliveryRepository) (MessageSender, []*domain.ContactWithList, map[string]*domain.Template, *domain.EmailProvider) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	createdAt := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	broadcast := &domain.Broadcast{
		ID: "broadcast-1", WorkspaceID: "workspace-1", Name: "Stable delivery", CreatedAt: createdAt,
		Audience: domain.AudienceSettings{List: "list-1"}, UTMParameters: &domain.UTMParameters{},
		TestSettings: domain.BroadcastTestSettings{Enabled: true, Variations: []domain.BroadcastVariation{
			{TemplateID: "template-a"}, {TemplateID: "template-b"},
		}},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-1", "broadcast-1").Return(broadcast, nil).AnyTimes()

	senderIdentity := domain.NewEmailSender("sender@example.com", "Sender")
	makeTemplate := func(id string, version int64) *domain.Template {
		return &domain.Template{ID: id, Version: version, Email: &domain.EmailTemplate{
			SenderID: senderIdentity.ID, Subject: "Hello",
			VisualEditorTree: createQueueValidTestTree(createQueueTestTextBlock("text-"+id, "Welcome")),
		}}
	}
	templates := map[string]*domain.Template{
		"template-a": makeTemplate("template-a", 3),
		"template-b": makeTemplate("template-b", 4),
	}
	provider := &domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{senderIdentity}}
	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "one@example.com"}, ListID: "list-1", SnapshotOrdinal: 10, DeliveryPhase: "test"},
		{Contact: &domain.Contact{Email: "two@example.com"}, ListID: "list-1", SnapshotOrdinal: 11, DeliveryPhase: "test"},
	}
	return NewQueueMessageSenderWithDelivery(
		mockQueueRepo, deliveryRepo, mockBroadcastRepo, mockHistoryRepo, mockTemplateRepo,
		nil, mockLogger, nil, "https://api.example.com",
	), recipients, templates, provider
}

func TestQueueMessageSenderDeliveryReplayKeepsEffectKeyVariantAndQueueIdentity(t *testing.T) {
	deliveryRepo := &captureBroadcastDeliveryRepository{}
	sender, recipients, templates, provider := deliveryQueueSenderFixture(t, deliveryRepo)

	for run := 0; run < 2; run++ {
		sent, failed, err := sender.SendBatch(
			context.Background(), "workspace-1", "integration-1", "secret",
			"https://api.example.com", "", false, nil, "broadcast-1", recipients,
			templates, provider, time.Now().Add(time.Minute), "",
		)
		require.NoError(t, err)
		assert.Equal(t, 2, sent)
		assert.Zero(t, failed)
	}

	require.Len(t, deliveryRepo.reservations, 4)
	for index := 0; index < 2; index++ {
		first, replay := deliveryRepo.reservations[index], deliveryRepo.reservations[index+2]
		assert.Equal(t, first.intent.EffectKey, replay.intent.EffectKey)
		assert.Equal(t, first.intent.RequestHash, replay.intent.RequestHash)
		assert.Equal(t, first.intent.Variant, replay.intent.Variant)
		assert.Equal(t, first.entry.ID, replay.entry.ID)
		assert.Equal(t, first.entry.MessageID, replay.entry.MessageID)
		assert.NotEmpty(t, first.intent.CustomerID)
		assert.Equal(t, "test", first.intent.NodeOrPhase)
	}
	assert.NotEqual(t, deliveryRepo.reservations[0].intent.EffectKey, deliveryRepo.reservations[1].intent.EffectKey)
}

func TestQueueMessageSenderDeliveryFailureLeavesWholeCursorForSafeReplay(t *testing.T) {
	deliveryRepo := &captureBroadcastDeliveryRepository{failAt: 2}
	sender, recipients, templates, provider := deliveryQueueSenderFixture(t, deliveryRepo)

	sent, failed, err := sender.SendBatch(
		context.Background(), "workspace-1", "integration-1", "secret",
		"https://api.example.com", "", false, nil, "broadcast-1", recipients,
		templates, provider, time.Now().Add(time.Minute), "",
	)
	assert.ErrorContains(t, err, "failed to reserve recipient delivery")
	assert.Zero(t, sent)
	assert.Zero(t, failed)
	assert.Len(t, deliveryRepo.reservations, 2)
}
