package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/broker"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type configuredQueueStub struct {
	jobs     []domain.ConfiguredEventJob
	finished []string
}

func (q *configuredQueueStub) ClaimConfiguredEvents(context.Context, string, time.Time, time.Duration, int) ([]domain.ConfiguredEventJob, error) {
	return q.jobs, nil
}
func (q *configuredQueueStub) FinishConfiguredEvent(_ context.Context, _ string, _ domain.ConfiguredEventJob, _ time.Time, reason string) error {
	q.finished = append(q.finished, reason)
	return nil
}

type configuredHandlerStub struct {
	err   error
	calls int
}

func (h *configuredHandlerStub) Handle(context.Context, broker.Message) error {
	h.calls++
	return h.err
}
func TestConfiguredEventWorkerKeepsFailedJobRetryable(t *testing.T) {
	q := &configuredQueueStub{jobs: []domain.ConfiguredEventJob{{ID: uuid.New(), Token: uuid.New(), Payload: []byte(`{}`)}}}
	h := &configuredHandlerStub{err: errors.New("database busy")}
	w := NewConfiguredEventWorker(stateWorkspaceCursor{workspaces: []string{"ws"}}, q, h, nil)
	count, err := w.ProcessOnce(context.Background())
	require.Error(t, err)
	require.Zero(t, count)
	require.Equal(t, []string{"database busy"}, q.finished)
	h.err = nil
	count, err = w.ProcessOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "", q.finished[1])
	require.Equal(t, 2, h.calls)
}
