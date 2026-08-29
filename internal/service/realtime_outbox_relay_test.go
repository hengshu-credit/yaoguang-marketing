package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
)

type rotatingWorkspaceCursor struct {
	ids  []string
	next int
	err  error
}

func (c *rotatingWorkspaceCursor) NextWorkspaceIDs(_ context.Context, _ string, limit int) ([]string, error) {
	if c.err != nil || len(c.ids) == 0 {
		return nil, c.err
	}
	count := min(limit, len(c.ids))
	result := make([]string, 0, count)
	for offset := 0; offset < count; offset++ {
		result = append(result, c.ids[(c.next+offset)%len(c.ids)])
	}
	c.next = (c.next + count) % len(c.ids)
	return result, nil
}

type relayPublisher struct {
	messages []broker.Message
	failFor  map[uuid.UUID]error
}

func (p *relayPublisher) Publish(_ context.Context, message broker.Message) error {
	p.messages = append(p.messages, message)
	return p.failFor[message.ID]
}

func TestOutboxRelayRotatesWorkspaceCursorAfterBusyTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockRealtimeRepository(ctrl)
	publisher := &relayPublisher{}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

	messageA := outboxFixture(uuid.New(), 1)
	messageB := outboxFixture(uuid.New(), 1)
	gomock.InOrder(
		repository.EXPECT().ClaimOutbox(gomock.Any(), "a", "relay-1", now, time.Minute, 1).
			Return([]domain.OutboxMessage{messageA}, nil),
		repository.EXPECT().MarkOutboxPublished(gomock.Any(), "a", messageA.ID, *messageA.ClaimToken, now).
			Return(true, nil),
		repository.EXPECT().ClaimOutbox(gomock.Any(), "b", "relay-1", now, time.Minute, 1).
			Return([]domain.OutboxMessage{messageB}, nil),
		repository.EXPECT().MarkOutboxPublished(gomock.Any(), "b", messageB.ID, *messageB.ClaimToken, now).
			Return(true, nil),
	)

	relay, err := NewOutboxRelay(
		&rotatingWorkspaceCursor{ids: []string{"a", "b", "c"}},
		repository, publisher, "relay-1", 1, time.Minute,
	)
	require.NoError(t, err)
	relay.now = func() time.Time { return now }

	processed, err := relay.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	processed, err = relay.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	require.Len(t, publisher.messages, 2)
	assert.Equal(t, messageA.ID, publisher.messages[0].ID)
	assert.Equal(t, messageB.ID, publisher.messages[1].ID)
}

func TestOutboxRelayConfirmFailureReleasesOriginalClaimWithBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockRealtimeRepository(ctrl)
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	message := outboxFixture(uuid.New(), 3)
	publishErr := errors.New("publisher confirm lost")
	publisher := &relayPublisher{failFor: map[uuid.UUID]error{message.ID: publishErr}}

	repository.EXPECT().ClaimOutbox(gomock.Any(), "workspace1", "relay-1", now, time.Minute, 10).
		Return([]domain.OutboxMessage{message}, nil)
	repository.EXPECT().ReleaseOutbox(
		gomock.Any(), "workspace1", message.ID, *message.ClaimToken,
		now.Add(7*time.Second), publishErr.Error(), false,
	).Return(true, nil)

	relay, err := NewOutboxRelay(
		&rotatingWorkspaceCursor{ids: []string{"workspace1"}},
		repository, publisher, "relay-1", 10, time.Minute,
	)
	require.NoError(t, err)
	relay.now = func() time.Time { return now }
	relay.retryBackoff = func(int, uuid.UUID) time.Duration { return 7 * time.Second }

	processed, err := relay.ProcessOnce(context.Background())
	assert.Equal(t, 1, processed)
	require.ErrorIs(t, err, publishErr)
	require.Len(t, publisher.messages, 1)
	assert.Equal(t, message.ID, publisher.messages[0].ID, "message identity must survive every retry")
}

func TestOutboxRelayInjectsWorkspaceOnlyIntoPublishedCopy(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockRealtimeRepository(ctrl)
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	message := outboxFixture(uuid.New(), 1)
	originalPayload := append([]byte(nil), message.Payload...)
	publisher := &relayPublisher{}

	repository.EXPECT().ClaimOutbox(gomock.Any(), "tenant42", "relay-1", now, time.Minute, 5).
		Return([]domain.OutboxMessage{message}, nil)
	repository.EXPECT().MarkOutboxPublished(gomock.Any(), "tenant42", message.ID, *message.ClaimToken, now).
		Return(true, nil)

	relay, err := NewOutboxRelay(
		&rotatingWorkspaceCursor{ids: []string{"tenant42"}},
		repository, publisher, "relay-1", 5, time.Minute,
	)
	require.NoError(t, err)
	relay.now = func() time.Time { return now }

	processed, err := relay.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.JSONEq(t, string(originalPayload), string(message.Payload), "repository value must remain immutable")

	require.Len(t, publisher.messages, 1)
	var published map[string]any
	require.NoError(t, json.Unmarshal(publisher.messages[0].Body, &published))
	assert.Equal(t, "tenant42", published["workspace_id"])
	assert.Equal(t, "tenant42", publisher.messages[0].Headers["workspace_id"])
}

func TestOutboxRelayMarksExhaustedMessageDead(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := mocks.NewMockRealtimeRepository(ctrl)
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	message := outboxFixture(uuid.New(), defaultOutboxMaxAttempts)
	publishErr := errors.New("broker unavailable")
	publisher := &relayPublisher{failFor: map[uuid.UUID]error{message.ID: publishErr}}

	repository.EXPECT().ClaimOutbox(gomock.Any(), "tenant", "relay-1", now, time.Minute, 5).
		Return([]domain.OutboxMessage{message}, nil)
	repository.EXPECT().ReleaseOutbox(
		gomock.Any(), "tenant", message.ID, *message.ClaimToken, now, publishErr.Error(), true,
	).Return(true, nil)

	relay, err := NewOutboxRelay(
		&rotatingWorkspaceCursor{ids: []string{"tenant"}},
		repository, publisher, "relay-1", 5, time.Minute,
	)
	require.NoError(t, err)
	relay.now = func() time.Time { return now }

	_, err = relay.ProcessOnce(context.Background())
	require.ErrorIs(t, err, publishErr)
}

func outboxFixture(messageID uuid.UUID, attempts int) domain.OutboxMessage {
	claimToken := uuid.New()
	correlationID := uuid.New()
	payload := []byte(`{
		"id":"` + messageID.String() + `",
		"event_id":"` + uuid.New().String() + `",
		"type":"contact.updated",
		"schema_version":1,
		"workspace_id":"",
		"correlation_id":"` + correlationID.String() + `",
		"data":{"score":42}
	}`)
	return domain.OutboxMessage{
		ID:         messageID,
		EventID:    uuid.New(),
		Topic:      broker.EventsExchange,
		RoutingKey: "contact.updated",
		Payload:    payload,
		Headers:    json.RawMessage(`{"schema_version":1,"trace_id":"trace-1"}`),
		Attempts:   attempts,
		ClaimToken: &claimToken,
	}
}
