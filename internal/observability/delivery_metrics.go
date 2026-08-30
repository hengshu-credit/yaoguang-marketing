package observability

import (
	"context"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	deliveryChannelKey, _  = tag.NewKey("channel")
	deliveryProviderKey, _ = tag.NewKey("provider")
	deliveryStatusKey, _   = tag.NewKey("status")

	deliveryOutcomeCount   = stats.Int64("yaoguang/delivery/outcomes", "Persisted delivery outcomes", stats.UnitDimensionless)
	deliveryAttemptLatency = stats.Float64("yaoguang/delivery/attempt_latency_ms", "Claim-to-outcome latency", stats.UnitMilliseconds)
	deliveryUnknownCount   = stats.Int64("yaoguang/delivery/unknown", "Uncertain provider outcomes", stats.UnitDimensionless)

	DeliveryOutcomeView = &view.View{
		Name: "yaoguang_delivery_outcomes", Measure: deliveryOutcomeCount,
		TagKeys: []tag.Key{deliveryChannelKey, deliveryProviderKey, deliveryStatusKey}, Aggregation: view.Count(),
	}
	DeliveryAttemptLatencyView = &view.View{
		Name: "yaoguang_delivery_attempt_latency_ms", Measure: deliveryAttemptLatency,
		TagKeys:     []tag.Key{deliveryChannelKey, deliveryProviderKey},
		Aggregation: view.Distribution(1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000),
	}
	DeliveryUnknownView = &view.View{
		Name: "yaoguang_delivery_unknown", Measure: deliveryUnknownCount,
		TagKeys: []tag.Key{deliveryChannelKey, deliveryProviderKey}, Aggregation: view.Count(),
	}
	registerDeliveryViewsOnce sync.Once
	registerDeliveryViewsErr  error
)

func RegisterDeliveryViews() error {
	registerDeliveryViewsOnce.Do(func() {
		registerDeliveryViewsErr = view.Register(DeliveryOutcomeView, DeliveryAttemptLatencyView, DeliveryUnknownView)
	})
	return registerDeliveryViewsErr
}

// RecordDeliveryOutcome deliberately excludes Workspace ID from tags to avoid
// unbounded Prometheus cardinality. Workspace remains available in structured logs.
func RecordDeliveryOutcome(ctx context.Context, channel, provider, status string, latency time.Duration) {
	_ = RegisterDeliveryViews()
	ctx, err := tag.New(ctx, tag.Upsert(deliveryChannelKey, channel), tag.Upsert(deliveryProviderKey, provider), tag.Upsert(deliveryStatusKey, status))
	if err != nil {
		return
	}
	measurements := []stats.Measurement{deliveryOutcomeCount.M(1), deliveryAttemptLatency.M(float64(latency.Milliseconds()))}
	if status == "unknown" {
		measurements = append(measurements, deliveryUnknownCount.M(1))
	}
	stats.Record(ctx, measurements...)
}
