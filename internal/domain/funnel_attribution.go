package domain

import "time"

// Touchpoints originate from platform delivery evidence and verified click IDs,
// not event data supplied by the caller. All linkage is workspace-local.
type AnalysisTouchpoint struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	CustomerID string    `json:"customer_id"`
	Kind       string    `json:"kind"`
	At         time.Time `json:"at"`
}
type AnalysisAttribution struct {
	RunID        string `json:"run_id,omitempty"`
	TouchpointID string `json:"touchpoint_id,omitempty"`
	Model        string `json:"model,omitempty"`
}

func AttributeAnalysisEvent(event AnalysisEvent, links []AnalysisTouchpoint, window int64) AnalysisAttribution {
	result := AnalysisAttribution{}
	var last time.Time
	ambiguous := false
	for _, link := range links {
		if link.CustomerID != event.CustomerID || link.At.After(event.OccurredAt) || event.OccurredAt.Sub(link.At) > time.Duration(window)*time.Second {
			continue
		}
		if event.TrustedTouchpointID != "" && link.ID == event.TrustedTouchpointID {
			return AnalysisAttribution{RunID: link.RunID, TouchpointID: link.ID, Model: "direct"}
		}
		if link.Kind == "click" {
			if link.At.After(last) {
				last = link.At
				ambiguous = false
				result = AnalysisAttribution{RunID: link.RunID, TouchpointID: link.ID, Model: "last_click"}
			} else if link.At.Equal(last) && result.RunID != link.RunID {
				ambiguous = true
			}
		}
	}
	if ambiguous {
		return AnalysisAttribution{}
	}
	return result
}
func FilterAnalysisScope(events []AnalysisEvent, links []AnalysisTouchpoint, scope, runID string, window int64) []AnalysisEvent {
	out := []AnalysisEvent{}
	for _, event := range events {
		event.Attribution = AttributeAnalysisEvent(event, links, window)
		include := scope == "" || scope == "all_site"
		if scope == "attributed" {
			include = event.Attribution.RunID == runID && runID != ""
		}
		if scope == "recipients" {
			for _, link := range links {
				if link.RunID == runID && link.CustomerID == event.CustomerID && !link.At.After(event.OccurredAt) && event.OccurredAt.Sub(link.At) <= time.Duration(window)*time.Second {
					include = true
					break
				}
			}
		}
		if include {
			out = append(out, event)
		}
	}
	return out
}
