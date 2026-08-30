package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerReconciliationRequestRequiresWorkspace(t *testing.T) {
	request := CustomerReconciliationRequest{JobType: CustomerReconciliationScan}
	require.Error(t, request.Validate())

	request.WorkspaceID = " workspace-1 "
	require.NoError(t, request.Validate())
	assert.Equal(t, "workspace-1", request.WorkspaceID)
}

func TestCustomerReconciliationRequestRejectsUnknownJobType(t *testing.T) {
	request := CustomerReconciliationRequest{WorkspaceID: "workspace-1", JobType: "overwrite"}
	assert.Error(t, request.Validate())
}

func TestCustomerReconciliationGetRequestValidatesRunID(t *testing.T) {
	request := CustomerReconciliationGetRequest{WorkspaceID: "workspace-1", RunID: "not-a-uuid"}
	assert.Error(t, request.Validate())
}
