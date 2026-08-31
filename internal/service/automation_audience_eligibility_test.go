package service

import (
	"context"
	"errors"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type automationAudienceEligibilityStub struct {
	result bool
	err    error
	calls  int
}

func (s *automationAudienceEligibilityStub) MatchesCustomerInternal(context.Context, string, string, int, string) (bool, error) {
	s.calls++
	return s.result, s.err
}

func TestAutomationAudienceEligibilitySkipsOnlyTheCurrentTouchAndKeepsNextNode(t *testing.T) {
	checker := &automationAudienceEligibilityStub{result: false}
	executor := &AutomationExecutor{audienceEligibility: checker}
	next := "delay-2"
	node := &domain.AutomationNode{ID: "email-1", Type: domain.NodeTypeEmail, NextNodeID: &next}
	journey := &domain.ContactAutomation{Context: map[string]interface{}{
		audienceContextAudienceID: "audience-1", audienceContextVersion: float64(7),
		audienceContextCustomerID: "customer-1", audienceContextBuildID: "build-7",
	}}

	skip, err := executor.shouldSkipAudienceTouch(context.Background(), "workspace-1", journey, node)
	require.NoError(t, err)
	assert.True(t, skip)
	result := audienceTouchSkipResult(node)
	require.NotNil(t, result.NextNodeID)
	assert.Equal(t, next, *result.NextNodeID)
	assert.Equal(t, domain.ContactAutomationStatusActive, result.Status)
	assert.Equal(t, true, result.Output["skipped"])
	assert.Equal(t, "audience_no_longer_matched", result.Output["skip_reason"])
}

func TestAutomationAudienceEligibilityDoesNotGateNonTouchNodes(t *testing.T) {
	checker := &automationAudienceEligibilityStub{result: false}
	executor := &AutomationExecutor{audienceEligibility: checker}
	journey := &domain.ContactAutomation{Context: map[string]interface{}{audienceContextAudienceID: "audience-1"}}

	skip, err := executor.shouldSkipAudienceTouch(context.Background(), "workspace-1", journey, &domain.AutomationNode{Type: domain.NodeTypeDelay})
	require.NoError(t, err)
	assert.False(t, skip)
	assert.Zero(t, checker.calls)
}

func TestAutomationAudienceEligibilityErrorsRetryTheNode(t *testing.T) {
	checker := &automationAudienceEligibilityStub{err: errors.New("database unavailable")}
	executor := &AutomationExecutor{audienceEligibility: checker}
	journey := &domain.ContactAutomation{Context: map[string]interface{}{
		audienceContextAudienceID: "audience-1", audienceContextVersion: 7,
		audienceContextCustomerID: "customer-1", audienceContextBuildID: "build-7",
	}}

	_, err := executor.shouldSkipAudienceTouch(context.Background(), "workspace-1", journey, &domain.AutomationNode{Type: domain.NodeTypePush})
	assert.ErrorContains(t, err, "database unavailable")
}
