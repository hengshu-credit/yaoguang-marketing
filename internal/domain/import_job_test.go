package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportCountersPreserveEveryAcceptedRow(t *testing.T) {
	counters := ImportCounters{Total: 3, Pending: 3}
	processing, err := counters.Transition(ImportRowPending, ImportRowProcessing)
	require.NoError(t, err)
	succeeded, err := processing.Transition(ImportRowProcessing, ImportRowSucceeded)
	require.NoError(t, err)
	assert.Equal(t, int64(3), succeeded.Pending+succeeded.Processing+succeeded.Succeeded+succeeded.Failed)
	assert.Equal(t, int64(1), succeeded.Succeeded)
}

func TestImportCountersRejectLostRows(t *testing.T) {
	assert.ErrorContains(t, (ImportCounters{Total: 10, Pending: 9}).Validate(), "conservation")
	_, err := (ImportCounters{Total: 1, Pending: 1}).Transition(ImportRowProcessing, ImportRowSucceeded)
	assert.Error(t, err)
}
