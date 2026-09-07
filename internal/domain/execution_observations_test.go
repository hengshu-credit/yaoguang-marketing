package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutionObservationsKeepOccurrencesAndSeparateAttribution(t *testing.T) {
	at := time.Now().UTC()
	plan := MarketingTrackingPlan{SubjectType: "loan", Events: []string{"borrow"}, States: []string{"approved"}, WindowSeconds: 3600}
	events := []AnalysisEvent{{ID: "1", Code: "borrow", SubjectType: "loan", SubjectID: "a", CustomerID: "customer", OccurredAt: at.Add(time.Minute)}, {ID: "2", Code: "borrow", SubjectType: "loan", SubjectID: "a", CustomerID: "customer", OccurredAt: at.Add(2 * time.Minute)}, {ID: "3", Code: "borrow", SubjectType: "loan", SubjectID: "other", CustomerID: "customer", OccurredAt: at.Add(time.Minute)}}
	cohort := []ExecutionObservationSubject{{CustomerID: "customer", SubjectType: "loan", SubjectID: "a", Since: at}}
	result := SummarizeExecutionObservations("run", plan, cohort, events, nil, nil, at.Add(time.Hour))
	require.Equal(t, 2, result.Events[0].Observed.Occurrences)
	require.Equal(t, 1, result.Events[0].Observed.Customers)
	require.Equal(t, 1, result.Events[0].Observed.Objects)
	require.Zero(t, result.Events[0].Attributed.Occurrences)
	links := []AnalysisTouchpoint{{ID: "click", RunID: "run", CustomerID: "customer", Kind: "click", At: at}}
	result = SummarizeExecutionObservations("run", plan, cohort, events, nil, links, at.Add(time.Hour))
	require.Equal(t, 2, result.Events[0].Attributed.Occurrences)
}
