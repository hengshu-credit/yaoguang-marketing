package service

import (
	"context"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ConfigurableAutomationValidator struct {
	base        domain.AutomationNodeValidator
	definitions domain.BusinessDefinitionRepository
}

func NewConfigurableAutomationValidator(base domain.AutomationNodeValidator, definitions domain.BusinessDefinitionRepository) *ConfigurableAutomationValidator {
	return &ConfigurableAutomationValidator{base, definitions}
}

func (v *ConfigurableAutomationValidator) ValidateAutomationNodes(ctx context.Context, w string, a *domain.Automation) error {
	if a == nil {
		return domain.NewValidationError("automation is required")
	}
	if v.base != nil {
		if err := v.base.ValidateAutomationNodes(ctx, w, a); err != nil {
			return err
		}
	}
	if a.Trigger == nil {
		return nil
	}
	check := func(kind, key string) (*domain.BusinessDefinition, error) {
		if kind == "object_type" && key == "customer" {
			return &domain.BusinessDefinition{Key: key, Status: "active"}, nil
		}
		d, err := v.definitions.Get(ctx, w, kind, key)
		if err != nil || d.Status != "active" {
			return nil, domain.NewValidationError(fmt.Sprintf("%s %s must be active", kind, key))
		}
		return d, nil
	}
	checkEvents := func(subject string, events []string) error {
		if _, err := check("object_type", subject); err != nil {
			return err
		}
		for _, code := range events {
			d, err := check("event", code)
			if err != nil {
				return err
			}
			typ, _ := d.Settings["subject_type"].(string)
			if typ == "" {
				typ = "customer"
			}
			if subject != "customer" && typ != subject && !(typ == "any" && d.Settings["builtin"] == true) {
				return domain.NewValidationError(fmt.Sprintf("event %s subject type does not match %s", code, subject))
			}
		}
		return nil
	}
	var checkRule func(domain.StateRule, string) error
	checkRule = func(rule domain.StateRule, subject string) error {
		if rule.Kind == "object_any" {
			subject = rule.SubjectType
			if _, err := check("object_type", subject); err != nil {
				return err
			}
		}
		if rule.Kind == "event" {
			if err := checkEvents(subject, []string{rule.Event}); err != nil {
				return err
			}
			if rule.RevokeEvent != "" {
				if err := checkEvents(subject, []string{rule.RevokeEvent}); err != nil {
					return err
				}
			}
		}
		if rule.Kind == "state" {
			d, err := check("state", rule.StateKey)
			if err != nil {
				return err
			}
			if d.Settings["subject_type"] != subject {
				return domain.NewValidationError("state subject type does not match event rule")
			}
		}
		for _, child := range rule.Children {
			if err := checkRule(child, subject); err != nil {
				return err
			}
		}
		return nil
	}
	if rule := a.Trigger.EventRule; rule != nil {
		if err := checkEvents(rule.SubjectType, rule.Events); err != nil {
			return err
		}
		if rule.Condition != nil {
			if err := checkRule(*rule.Condition, rule.SubjectType); err != nil {
				return err
			}
		}
	}
	if plan := a.Trigger.TrackingPlan; plan != nil {
		if err := plan.Validate(); err != nil {
			return domain.NewValidationError(err.Error())
		}
		if err := checkEvents(plan.SubjectType, append(append([]string{}, plan.Events...), plan.GoalEvents...)); err != nil {
			return err
		}
		for _, key := range plan.States {
			d, err := check("state", key)
			if err != nil {
				return err
			}
			if d.Settings["subject_type"] != plan.SubjectType {
				return domain.NewValidationError("tracked state subject type does not match tracking plan")
			}
		}
	}
	for _, node := range a.Nodes {
		if node == nil || node.Type != domain.NodeTypeBusinessCondition {
			continue
		}
		condition, err := domain.ParseBusinessCondition(node.Config)
		if err != nil {
			return domain.NewValidationError(fmt.Sprintf("node %s: %v", node.ID, err))
		}
		if err := checkEvents(condition.SubjectType, condition.Events); err != nil {
			return err
		}
		if condition.SubjectType != "customer" && (a.Trigger.EventRule == nil || a.Trigger.EventRule.SubjectType != condition.SubjectType) {
			return domain.NewValidationError("business condition must use the enrolled object type")
		}
		if condition.Condition != nil {
			if err := checkRule(*condition.Condition, condition.SubjectType); err != nil {
				return err
			}
		}
	}
	return nil
}
