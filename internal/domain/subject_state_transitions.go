package domain

import "context"

type StateTransition struct {
	EventType string
	Previous  StateValue
	Current   SubjectStateInterval
}
type StateTransitionEmission struct {
	Enabled      bool
	AllowInitial bool
	Cause        *EventEnvelope
	Depth        int
}
type stateTransitionEmissionKey struct{}

func WithStateTransitionEmission(ctx context.Context, emission StateTransitionEmission) context.Context {
	return context.WithValue(ctx, stateTransitionEmissionKey{}, emission)
}
func StateTransitionEmissionFrom(ctx context.Context) StateTransitionEmission {
	v, _ := ctx.Value(stateTransitionEmissionKey{}).(StateTransitionEmission)
	return v
}
func SubjectStateTransitions(previous, current []SubjectStateInterval, allowInitial bool) []StateTransition {
	old := map[string]SubjectStateInterval{}
	for _, h := range previous {
		if h.ValidTo == nil {
			old[h.StateKey] = h
		}
	}
	out := []StateTransition{}
	for _, h := range current {
		if h.ValidTo != nil {
			continue
		}
		before, exists := old[h.StateKey]
		if !exists {
			if allowInitial && h.Value == StateTrue {
				out = append(out, StateTransition{EventType: "state.entered", Previous: StateUnknown, Current: h})
			}
			continue
		}
		if before.RuleRevision != h.RuleRevision || before.Value == h.Value {
			continue
		}
		if h.Value == StateTrue {
			out = append(out, StateTransition{EventType: "state.entered", Previous: before.Value, Current: h})
		} else if before.Value == StateTrue {
			out = append(out, StateTransition{EventType: "state.exited", Previous: before.Value, Current: h})
		}
	}
	return out
}
