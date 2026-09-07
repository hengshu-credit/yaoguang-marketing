package service

import (
	"context"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (s *BusinessConfigService) analysisReference(ctx context.Context, w, kind, key, subject string) error {
	ref, err := s.repo.Get(ctx, w, kind, key)
	if err != nil || ref == nil || ref.Status != "active" {
		return fmt.Errorf("%s %q must be active", kind, key)
	}
	typ, _ := ref.Settings["subject_type"].(string)
	if subject != "customer" && typ != "" && typ != "any" && typ != subject {
		return fmt.Errorf("%s %q belongs to another object type", kind, key)
	}
	return nil
}
func (s *BusinessConfigService) analysisRelation(ctx context.Context, w, key, to string, from map[string]bool) error {
	ref, err := s.repo.Get(ctx, w, "object_relation", key)
	if err != nil || ref == nil || ref.Status != "active" {
		return fmt.Errorf("object relation %q must be active", key)
	}
	a, _ := ref.Settings["from_type"].(string)
	b, _ := ref.Settings["to_type"].(string)
	if !from[a] || b != to {
		return fmt.Errorf("relation %q must connect a bound parent to %q", key, to)
	}
	return nil
}
func (s *BusinessConfigService) validateAnalysisRuleReferences(ctx context.Context, w, subject string, rule domain.StateRule) error {
	if rule.Kind == "event" {
		for _, key := range []string{rule.Event, rule.RevokeEvent} {
			if key != "" {
				if err := s.analysisReference(ctx, w, "event", key, subject); err != nil {
					return err
				}
			}
		}
	}
	if rule.Kind == "state" {
		if err := s.analysisReference(ctx, w, "state", rule.StateKey, subject); err != nil {
			return err
		}
	}
	if rule.Kind == "object_any" {
		if err := s.analysisReference(ctx, w, "object_type", rule.SubjectType, rule.SubjectType); err != nil {
			return err
		}
		if rule.RelationKey != "" {
			if err := s.analysisRelation(ctx, w, rule.RelationKey, rule.SubjectType, map[string]bool{subject: true}); err != nil {
				return err
			}
		} else if subject != "customer" {
			return fmt.Errorf("object aggregate requires a relation for a non-customer parent")
		}
		subject = rule.SubjectType
	}
	for _, child := range rule.Children {
		if err := s.validateAnalysisRuleReferences(ctx, w, subject, child); err != nil {
			return err
		}
	}
	return nil
}
func (s *BusinessConfigService) validateFunnelReferences(ctx context.Context, w string, d domain.FunnelDefinition) error {
	bound := map[string]bool{d.SubjectType: true}
	for _, step := range d.Steps {
		typ := step.SubjectType
		if typ == "" {
			typ = d.SubjectType
		}
		if typ != "customer" {
			if err := s.analysisReference(ctx, w, "object_type", typ, typ); err != nil {
				return err
			}
		}
		if step.RelationKey != "" {
			if err := s.analysisRelation(ctx, w, step.RelationKey, typ, bound); err != nil {
				return err
			}
		}
		bound[typ] = true
		for _, event := range append(append([]string(nil), step.Events...), step.AllEvents...) {
			if err := s.analysisReference(ctx, w, "event", event, typ); err != nil {
				return err
			}
		}
		for key := range step.StateFilters {
			if err := s.analysisReference(ctx, w, "state", key, typ); err != nil {
				return err
			}
		}
		if step.Filter != nil {
			if err := s.validateAnalysisRuleReferences(ctx, w, typ, *step.Filter); err != nil {
				return err
			}
		}
	}
	return nil
}
