package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats/view"
)

func TestDeliveryMetricsUseBoundedOperationalTags(t *testing.T) {
	require.NoError(t, RegisterDeliveryViews())
	assert.Len(t, DeliveryOutcomeView.TagKeys, 3)
	for _, key := range DeliveryOutcomeView.TagKeys {
		assert.NotEqual(t, "workspace_id", key.Name())
	}
	RecordDeliveryOutcome(context.Background(), "email", "ses", "unknown", 12*time.Millisecond)
	rows, err := view.RetrieveData(DeliveryUnknownView.Name)
	require.NoError(t, err)
	assert.NotEmpty(t, rows)
}
