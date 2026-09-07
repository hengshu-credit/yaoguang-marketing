package repository

import (
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func applyTrackedEventCutoff(event *domain.TrackedEvent, now time.Time) {
	if event.Backfill {
		event.ProcessingStatus = "backfill"
	} else if event.ExpiresAt != nil && !now.Before(*event.ExpiresAt) {
		event.ProcessingStatus = "expired"
	}
}
func trackedEventIsHistorical(event *domain.TrackedEvent) bool {
	return event.Backfill || event.ProcessingStatus == "backfill" || event.ProcessingStatus == "expired" || event.ProcessingStatus == "associated_history"
}
