package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type AnalysisObjectRelation struct {
	Key       string
	FromType  string
	FromID    string
	ToType    string
	ToID      string
	ValidFrom time.Time
	ValidTo   *time.Time
}

func (d FunnelDefinition) HasLinkedSteps() bool {
	for _, s := range d.Steps {
		if s.SubjectType != "" && s.SubjectType != d.SubjectType {
			return true
		}
	}
	return false
}
func funnelSubjectKey(typ, id string) string { return typ + "\x00" + id }
func expandFunnelGroups(d FunnelDefinition, groups map[string][]AnalysisEvent, input []AnalysisEvent, relations []AnalysisObjectRelation) error {
	if d.SubjectType == "customer" {
		return nil
	}
	bySubject := map[string][]AnalysisEvent{}
	edges := map[string][]string{}
	for _, e := range input {
		key := funnelSubjectKey(e.SubjectType, e.SubjectID)
		bySubject[key] = append(bySubject[key], e)
	}
	for _, r := range relations {
		from := funnelSubjectKey(r.FromType, r.FromID)
		edges[from] = append(edges[from], funnelSubjectKey(r.ToType, r.ToID))
	}
	budget := 1000000
	for root := range groups {
		key := funnelSubjectKey(d.SubjectType, root)
		seen := map[string]bool{key: true}
		queue := []string{key}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, child := range edges[current] {
				budget--
				if budget < 0 {
					return fmt.Errorf("linked object query budget exceeded")
				}
				if !seen[child] {
					seen[child] = true
					queue = append(queue, child)
					groups[root] = append(groups[root], bySubject[child]...)
				}
			}
		}
	}
	return nil
}
func funnelBindingKey(bindings map[string]string) string {
	keys := make([]string, 0, len(bindings))
	for k, v := range bindings {
		keys = append(keys, k+"="+v)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}
func funnelEventInBinding(d FunnelDefinition, s FunnelStep, p funnelPath, e AnalysisEvent, relations []AnalysisObjectRelation) bool {
	typ := s.SubjectType
	if typ == "" {
		typ = d.SubjectType
	}
	if typ == "customer" {
		return e.CustomerID == p.bindings["customer"]
	}
	if e.SubjectType != typ {
		return false
	}
	if bound, ok := p.bindings[typ]; ok {
		return bound == e.SubjectID
	}
	if s.RelationKey == "" {
		return false
	}
	for _, r := range relations {
		if r.Key == s.RelationKey && r.ToType == typ && r.ToID == e.SubjectID && p.bindings[r.FromType] == r.FromID && !e.OccurredAt.Before(r.ValidFrom) && (r.ValidTo == nil || e.OccurredAt.Before(*r.ValidTo)) {
			return true
		}
	}
	return false
}
func bindFunnelEvent(d FunnelDefinition, s FunnelStep, p funnelPath, e AnalysisEvent) map[string]string {
	out := map[string]string{}
	for k, v := range p.bindings {
		out[k] = v
	}
	typ := s.SubjectType
	if typ == "" {
		typ = d.SubjectType
	}
	// A customer-scoped step observes an event without changing the chain's
	// previously selected application/order. Only an object-scoped step binds.
	if typ != "customer" {
		out[typ] = e.SubjectID
	}
	return out
}
