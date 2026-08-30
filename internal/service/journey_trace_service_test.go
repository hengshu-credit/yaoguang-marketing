package service

import (
	"context"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

type journeyTraceSourceStub struct {
	instances []domain.JourneyInstanceSummary
	total     int
	trace     *domain.JourneyTrace
	err       error
}

func (s journeyTraceSourceStub) ListJourneyInstances(context.Context, domain.JourneyInstanceListRequest) ([]domain.JourneyInstanceSummary, int, error) {
	return s.instances, s.total, s.err
}

func (s journeyTraceSourceStub) GetJourneyTrace(context.Context, domain.JourneyTraceRequest) (*domain.JourneyTrace, error) {
	return s.trace, s.err
}

func TestJourneyTraceSupportsCustomerLocatorAndReturnsExecutionLinks(t *testing.T) {
	want := &domain.JourneyTrace{
		Instance:   domain.JourneyInstanceSummary{JourneyInstance: domain.JourneyInstance{ID: "instance-1"}, CustomerID: "customer-1", OriginEventID: "event-1"},
		Events:     []domain.JourneyTraceEvent{{ID: "event-log-1", NodeID: "sms", EventType: "state_changed", Status: "completed"}},
		Deliveries: []domain.JourneyDeliveryLink{{Intent: domain.DeliveryIntent{ID: "intent-1"}, Attempts: []domain.DeliveryAttempt{{ID: "attempt-1"}}, Receipts: []domain.DeliveryReceiptLink{{ID: "sms:receipt-1"}}}},
	}
	service, err := NewJourneyTraceService(journeyTraceSourceStub{instances: []domain.JourneyInstanceSummary{want.Instance}, total: 1, trace: want}, nil)
	require.NoError(t, err)

	instances, total, err := service.ListInstances(context.Background(), domain.JourneyInstanceListRequest{WorkspaceID: "ws", Locator: domain.JourneyCustomerLocator{ExternalUserID: "crm-100"}, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "customer-1", instances[0].CustomerID)

	trace, err := service.GetTrace(context.Background(), domain.JourneyTraceRequest{WorkspaceID: "ws", JourneyInstanceID: "instance-1"})
	require.NoError(t, err)
	require.Equal(t, "event-1", trace.Instance.OriginEventID)
	require.Equal(t, "intent-1", trace.Deliveries[0].Intent.ID)
	require.Equal(t, "attempt-1", trace.Deliveries[0].Attempts[0].ID)
	require.Equal(t, "sms:receipt-1", trace.Deliveries[0].Receipts[0].ID)
}

func TestJourneyTraceReturnsNotFoundWithinWorkspaceBoundary(t *testing.T) {
	service, err := NewJourneyTraceService(journeyTraceSourceStub{err: domain.ErrJourneyTraceNotFound}, nil)
	require.NoError(t, err)
	_, err = service.GetTrace(context.Background(), domain.JourneyTraceRequest{WorkspaceID: "other-workspace", JourneyInstanceID: "instance-1"})
	require.ErrorIs(t, err, domain.ErrJourneyTraceNotFound)
}
