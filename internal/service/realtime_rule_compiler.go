package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

type TriggerBindingCompiler struct {
	queryBuilder *QueryBuilder
}

func NewTriggerBindingCompiler(queryBuilder *QueryBuilder) *TriggerBindingCompiler {
	return &TriggerBindingCompiler{queryBuilder: queryBuilder}
}

func (c *TriggerBindingCompiler) Compile(automation *domain.Automation) (domain.TriggerBinding, error) {
	if automation == nil || automation.Trigger == nil {
		return domain.TriggerBinding{}, fmt.Errorf("automation and trigger are required")
	}
	if automation.ID == "" || automation.RootNodeID == "" {
		return domain.TriggerBinding{}, fmt.Errorf("automation id and root node id are required")
	}
	if c == nil || c.queryBuilder == nil {
		return domain.TriggerBinding{}, fmt.Errorf("trigger binding query builder is required")
	}
	if err := automation.Trigger.Validate(); err != nil {
		return domain.TriggerBinding{}, fmt.Errorf("validate automation trigger: %w", err)
	}

	eventType, subjectType, dependencyKeys, err := compileTriggerIndex(automation.Trigger)
	if err != nil {
		return domain.TriggerBinding{}, err
	}
	query, arguments, err := c.compileCondition(automation.Trigger.Conditions)
	if err != nil {
		return domain.TriggerBinding{}, err
	}

	conditionIdentity, err := json.Marshal(struct {
		RootNodeID string                        `json:"root_node_id"`
		Trigger    *domain.TimelineTriggerConfig `json:"trigger"`
	}{RootNodeID: automation.RootNodeID, Trigger: automation.Trigger})
	if err != nil {
		return domain.TriggerBinding{}, fmt.Errorf("marshal trigger condition identity: %w", err)
	}
	conditionHash, err := domain.CanonicalJSONHash(conditionIdentity)
	if err != nil {
		return domain.TriggerBinding{}, err
	}

	compiled, err := json.Marshal(domain.CompiledTriggerCondition{
		Query:      query,
		Arguments:  arguments,
		RootNodeID: automation.RootNodeID,
		Frequency:  automation.Trigger.Frequency,
		Trigger:    automation.Trigger,
	})
	if err != nil {
		return domain.TriggerBinding{}, fmt.Errorf("marshal compiled trigger condition: %w", err)
	}

	return domain.TriggerBinding{
		AutomationID:      automation.ID,
		AutomationVersion: 1,
		EventType:         eventType,
		SubjectType:       subjectType,
		DependencyKeys:    dependencyKeys,
		ConditionHash:     conditionHash,
		CompiledCondition: compiled,
	}, nil
}

func (c *TriggerBindingCompiler) compileCondition(tree *domain.TreeNode) (string, []any, error) {
	if tree == nil {
		return "SELECT TRUE", nil, nil
	}
	_, arguments, err := c.queryBuilder.BuildTriggerCondition(tree, "''::text")
	if err != nil {
		return "", nil, fmt.Errorf("compile realtime trigger condition: %w", err)
	}
	emailPlaceholder := fmt.Sprintf("$%d", len(arguments)+1)
	condition, rebuiltArguments, err := c.queryBuilder.BuildTriggerCondition(tree, emailPlaceholder)
	if err != nil {
		return "", nil, fmt.Errorf("compile realtime trigger email condition: %w", err)
	}
	if len(rebuiltArguments) != len(arguments) {
		return "", nil, fmt.Errorf("trigger condition argument count changed while binding email")
	}
	return "SELECT COALESCE((" + condition + "), FALSE)", arguments, nil
}

func compileTriggerIndex(trigger *domain.TimelineTriggerConfig) (string, string, []string, error) {
	eventType := trigger.EventKind
	subjectType := ""
	dependencies := make([]string, 0, len(trigger.UpdatedFields)+1)

	switch {
	case trigger.EventKind == "custom_event":
		eventType = "custom_event." + *trigger.CustomEventName
		subjectType = "custom_event"
	case trigger.EventKind == "contact.tagged" || trigger.EventKind == "contact.untagged":
		subjectType = "contact_tag"
		dependencies = append(dependencies, "entity_id:"+*trigger.Tag)
	case trigger.EventKind == "contact.profile_created" || trigger.EventKind == "contact.profile_updated":
		subjectType = "contact_profile"
	case strings.HasPrefix(trigger.EventKind, "contact."):
		subjectType = "contact"
	case strings.HasPrefix(trigger.EventKind, "list."):
		subjectType = "contact_list"
		dependencies = append(dependencies, "entity_id:"+*trigger.ListID)
	case strings.HasPrefix(trigger.EventKind, "segment."):
		subjectType = "contact_segment"
		dependencies = append(dependencies, "entity_id:"+*trigger.SegmentID)
	case strings.HasPrefix(trigger.EventKind, "email."):
		subjectType = "message_history"
	default:
		return "", "", nil, fmt.Errorf("unsupported realtime trigger event kind %q", trigger.EventKind)
	}

	if trigger.EventKind == "contact.updated" {
		for _, field := range trigger.UpdatedFields {
			if !AllowedContactFields[field] {
				return "", "", nil, fmt.Errorf("invalid updated_field: %s", field)
			}
			dependencies = append(dependencies, "changes."+field)
		}
	}
	return eventType, subjectType, sortedUniqueStrings(dependencies), nil
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
