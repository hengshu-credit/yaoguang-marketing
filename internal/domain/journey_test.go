package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyEnrollmentKeyPreservesOnceAndEveryEventSemantics(t *testing.T) {
	onceV1, err := JourneyEnrollmentDedupeKey("automation-1", "customer-1", TriggerFrequencyOnce, "event-1")
	require.NoError(t, err)
	onceV9, err := JourneyEnrollmentDedupeKey("automation-1", "customer-1", TriggerFrequencyOnce, "event-99")
	require.NoError(t, err)
	assert.Equal(t, onceV1, onceV9, "once must ignore automation versions and origin events")

	everyA, err := JourneyEnrollmentDedupeKey("automation-1", "customer-1", TriggerFrequencyEveryTime, "event-1")
	require.NoError(t, err)
	everyReplay, err := JourneyEnrollmentDedupeKey("automation-1", "customer-1", TriggerFrequencyEveryTime, "event-1")
	require.NoError(t, err)
	everyB, err := JourneyEnrollmentDedupeKey("automation-1", "customer-1", TriggerFrequencyEveryTime, "event-2")
	require.NoError(t, err)
	assert.Equal(t, everyA, everyReplay)
	assert.NotEqual(t, everyA, everyB)
}

func TestJourneyEntryGuardIsOptionalAndSeparateFromMessageFrequency(t *testing.T) {
	guard := JourneyEntryGuard{}
	assert.False(t, guard.Enabled)
	require.NoError(t, guard.Validate())
	guard = JourneyEntryGuard{Enabled: true, Cooldown: time.Hour, MaxConcurrent: 2}
	require.NoError(t, guard.Validate())
	assert.Equal(t, time.Hour, guard.Cooldown)
}
