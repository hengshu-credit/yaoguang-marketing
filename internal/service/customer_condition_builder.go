package service

import (
	"fmt"
	"strings"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

var customerStringAttributeFields = map[string]struct{}{
	"first_name": {}, "last_name": {}, "phone": {},
	"address_line_1": {}, "address_line_2": {}, "country": {}, "postcode": {}, "state": {},
	"job_title":       {},
	"custom_string_1": {}, "custom_string_2": {}, "custom_string_3": {}, "custom_string_4": {}, "custom_string_5": {},
}

var customerNumberAttributeFields = map[string]struct{}{
	"custom_number_1": {}, "custom_number_2": {}, "custom_number_3": {}, "custom_number_4": {}, "custom_number_5": {},
}

var customerTimeAttributeFields = map[string]struct{}{
	"custom_datetime_1": {}, "custom_datetime_2": {}, "custom_datetime_3": {}, "custom_datetime_4": {}, "custom_datetime_5": {},
}

var customerJSONAttributeFields = map[string]struct{}{
	"custom_json_1": {}, "custom_json_2": {}, "custom_json_3": {}, "custom_json_4": {}, "custom_json_5": {},
}

func legacyContactColumn(field string) string {
	return fmt.Sprintf("(SELECT legacy_contact.%s FROM contacts legacy_contact WHERE legacy_contact.customer_id = customer.id ORDER BY legacy_contact.updated_at DESC LIMIT 1)", field)
}

func legacyProfileColumn(field string) string {
	return fmt.Sprintf("(SELECT legacy_profile.%s FROM contact_profiles legacy_profile JOIN contacts legacy_contact ON legacy_contact.email = legacy_profile.email WHERE legacy_contact.customer_id = customer.id ORDER BY legacy_profile.updated_at DESC LIMIT 1)", field)
}

func (qb *QueryBuilder) customerFieldConfig(field string) (fieldConfig, bool) {
	switch field {
	case "email":
		return fieldConfig{dbColumn: legacyContactColumn("email"), fieldType: "string"}, true
	case "external_id":
		return fieldConfig{dbColumn: "customer.external_user_id", fieldType: "string"}, true
	case "language":
		return fieldConfig{dbColumn: "COALESCE(profile.language, " + legacyContactColumn("language") + ")", fieldType: "string"}, true
	case "timezone":
		return fieldConfig{dbColumn: "COALESCE(profile.timezone, " + legacyContactColumn("timezone") + ")", fieldType: "string"}, true
	case "created_at":
		return fieldConfig{dbColumn: "customer.created_at", fieldType: "time"}, true
	case "updated_at":
		return fieldConfig{dbColumn: "customer.updated_at", fieldType: "time"}, true
	case "profile_status":
		return fieldConfig{dbColumn: "COALESCE(profile.status, " + legacyProfileColumn("status") + ")", fieldType: "string"}, true
	case "profile_attributes":
		return fieldConfig{dbColumn: "(COALESCE(" + legacyProfileColumn("attributes") + ", '{}'::jsonb) || COALESCE(profile.attributes, '{}'::jsonb))", fieldType: "json"}, true
	case "profile_tags":
		return fieldConfig{fieldType: "customer_tags"}, true
	}

	if _, ok := customerStringAttributeFields[field]; ok {
		return fieldConfig{
			dbColumn:  fmt.Sprintf("COALESCE(profile.attributes ->> '%s', %s)", field, legacyContactColumn(field)),
			fieldType: "string",
		}, true
	}
	if _, ok := customerNumberAttributeFields[field]; ok {
		canonical := fmt.Sprintf("profile.attributes ->> '%s'", field)
		return fieldConfig{
			dbColumn:  fmt.Sprintf("COALESCE(%s, %s)", castNumeric(canonical, castDefensively), legacyContactColumn(field)),
			fieldType: "number",
		}, true
	}
	if _, ok := customerTimeAttributeFields[field]; ok {
		canonical := fmt.Sprintf("profile.attributes ->> '%s'", field)
		return fieldConfig{
			dbColumn:  fmt.Sprintf("COALESCE(%s, %s)", castTimestamp(canonical, castDefensively), legacyContactColumn(field)),
			fieldType: "time",
		}, true
	}
	if _, ok := customerJSONAttributeFields[field]; ok {
		return fieldConfig{
			dbColumn:  fmt.Sprintf("COALESCE(profile.attributes -> '%s', %s)", field, legacyContactColumn(field)),
			fieldType: "json",
		}, true
	}
	return fieldConfig{}, false
}

func (qb *QueryBuilder) parseCustomerNode(node *domain.TreeNode, argIndex int) (string, []interface{}, int, error) {
	switch node.Kind {
	case "branch":
		if node.Branch == nil {
			return "", nil, argIndex, fmt.Errorf("branch cannot be nil")
		}
		conditions := make([]string, 0, len(node.Branch.Leaves))
		args := make([]interface{}, 0)
		for _, child := range node.Branch.Leaves {
			condition, childArgs, next, err := qb.parseCustomerNode(child, argIndex)
			if err != nil {
				return "", nil, argIndex, err
			}
			conditions = append(conditions, condition)
			args = append(args, childArgs...)
			argIndex = next
		}
		operator := " AND "
		if node.Branch.Operator == "or" {
			operator = " OR "
		}
		return "(" + strings.Join(conditions, operator) + ")", args, argIndex, nil
	case "leaf":
		return qb.parseCustomerLeaf(node.Leaf, argIndex)
	default:
		return "", nil, argIndex, fmt.Errorf("invalid node kind: %s", node.Kind)
	}
}

func (qb *QueryBuilder) parseCustomerLeaf(leaf *domain.TreeNodeLeaf, argIndex int) (string, []interface{}, int, error) {
	if leaf == nil {
		return "", nil, argIndex, fmt.Errorf("leaf cannot be nil")
	}
	switch leaf.Source {
	case "contacts":
		return qb.parseCustomerContactCondition(leaf.Contact, argIndex)
	case "contact_lists":
		return qb.parseCustomerListCondition(leaf.ContactList, argIndex)
	case "contact_timeline":
		return qb.parseCustomerTimelineCondition(leaf.ContactTimeline, argIndex)
	case "custom_events_goals":
		return qb.parseCustomerGoalCondition(leaf.CustomEventsGoal, argIndex)
	default:
		return "", nil, argIndex, fmt.Errorf("unsupported source: %s", leaf.Source)
	}
}

func (qb *QueryBuilder) parseCustomerContactCondition(contact *domain.ContactCondition, argIndex int) (string, []interface{}, int, error) {
	if contact == nil {
		return "", nil, argIndex, fmt.Errorf("contact condition cannot be nil")
	}
	conditions := make([]string, 0, len(contact.Filters))
	args := make([]interface{}, 0)
	for _, filter := range contact.Filters {
		if filter.FieldName == "profile_tags" {
			condition, filterArgs, next, err := qb.buildCustomerTagCondition(filter, argIndex)
			if err != nil {
				return "", nil, argIndex, err
			}
			conditions = append(conditions, condition)
			args = append(args, filterArgs...)
			argIndex = next
			continue
		}
		config, ok := qb.customerFieldConfig(filter.FieldName)
		if !ok {
			return "", nil, argIndex, fmt.Errorf("invalid customer field name: %s", filter.FieldName)
		}
		condition, filterArgs, next, err := qb.parseFilterWithConfig(filter, argIndex, castDefensively, config)
		if err != nil {
			return "", nil, argIndex, err
		}
		conditions = append(conditions, condition)
		args = append(args, filterArgs...)
		argIndex = next
	}
	return "(" + strings.Join(conditions, " AND ") + ")", args, argIndex, nil
}

func (qb *QueryBuilder) buildCustomerTagCondition(filter *domain.DimensionFilter, argIndex int) (string, []interface{}, int, error) {
	canonicalPrefix := "EXISTS (SELECT 1 FROM customer_tags tag WHERE tag.customer_id = customer.id"
	legacyPrefix := "EXISTS (SELECT 1 FROM contact_tags legacy_tag JOIN contacts legacy_contact ON legacy_contact.email = legacy_tag.email WHERE legacy_contact.customer_id = customer.id"
	switch filter.Operator {
	case "is_set":
		return "(" + canonicalPrefix + ") OR " + legacyPrefix + "))", nil, argIndex, nil
	case "is_not_set":
		return "NOT (" + canonicalPrefix + ") OR " + legacyPrefix + "))", nil, argIndex, nil
	case "equals", "not_equals", "in_array", "contains", "not_contains":
		values, err := qb.getStringValues(filter)
		if err != nil {
			return "", nil, argIndex, err
		}
		if len(values) != 1 {
			return "", nil, argIndex, fmt.Errorf("%s requires exactly one tag", filter.Operator)
		}
		value := values[0]
		comparison := fmt.Sprintf(" = $%d", argIndex)
		if filter.Operator == "contains" || filter.Operator == "not_contains" {
			comparison = fmt.Sprintf(" ILIKE $%d", argIndex)
			value = "%" + values[0].(string) + "%"
		}
		condition := "(" + canonicalPrefix + " AND tag.tag" + comparison + ") OR " + legacyPrefix + " AND legacy_tag.tag" + comparison + "))"
		if filter.Operator == "not_equals" || filter.Operator == "not_contains" {
			condition = "NOT " + condition
		}
		return condition, []interface{}{value}, argIndex + 1, nil
	default:
		return "", nil, argIndex, fmt.Errorf("invalid operator for profile_tags: %s", filter.Operator)
	}
}

func (qb *QueryBuilder) parseCustomerListCondition(condition *domain.ContactListCondition, argIndex int) (string, []interface{}, int, error) {
	if condition == nil {
		return "", nil, argIndex, fmt.Errorf("contact_list condition cannot be nil")
	}
	args := []interface{}{condition.ListID}
	predicates := []string{fmt.Sprintf("%%s.list_id = $%d", argIndex)}
	argIndex++
	if condition.Status != nil && *condition.Status != "" {
		args = append(args, *condition.Status)
		predicates = append(predicates, fmt.Sprintf("%%s.status = $%d", argIndex))
		argIndex++
	}
	for index := range predicates {
		predicates[index] = strings.ReplaceAll(predicates[index], "%s", "membership")
	}
	canonical := "EXISTS (SELECT 1 FROM customer_list_memberships membership JOIN lists list ON list.id = membership.list_id WHERE membership.customer_id = customer.id AND " + strings.Join(predicates, " AND ") + " AND list.deleted_at IS NULL)"
	legacyPredicates := make([]string, len(predicates))
	for index, predicate := range predicates {
		legacyPredicates[index] = strings.ReplaceAll(predicate, "membership.", "legacy_membership.")
	}
	legacy := "EXISTS (SELECT 1 FROM contact_lists legacy_membership JOIN lists legacy_list ON legacy_list.id = legacy_membership.list_id WHERE (legacy_membership.customer_id = customer.id OR (legacy_membership.customer_id IS NULL AND EXISTS (SELECT 1 FROM contacts legacy_contact WHERE legacy_contact.customer_id = customer.id AND legacy_contact.email = legacy_membership.email))) AND " + strings.Join(legacyPredicates, " AND ") + " AND legacy_membership.deleted_at IS NULL AND legacy_list.deleted_at IS NULL)"
	combined := "(" + canonical + " OR " + legacy + ")"
	if condition.Operator == "not_in" {
		combined = "NOT " + combined
	}
	return combined, args, argIndex, nil
}

func customerTimelineSubject(alias string) string {
	return fmt.Sprintf("(%s.customer_id = customer.id OR (%s.customer_id IS NULL AND EXISTS (SELECT 1 FROM contacts legacy_contact WHERE legacy_contact.customer_id = customer.id AND legacy_contact.email = %s.email)))", alias, alias, alias)
}

func (qb *QueryBuilder) parseCustomerTimelineCondition(timeline *domain.ContactTimelineCondition, argIndex int) (string, []interface{}, int, error) {
	if timeline == nil {
		return "", nil, argIndex, fmt.Errorf("contact_timeline condition cannot be nil")
	}
	args := []interface{}{timeline.Kind}
	conditions := []string{customerTimelineSubject("ct"), fmt.Sprintf("ct.kind = $%d", argIndex)}
	argIndex++
	if timeline.TimeframeOperator != nil && *timeline.TimeframeOperator != "" && *timeline.TimeframeOperator != "anytime" {
		condition, values, next, err := qb.parseTimeframeCondition(*timeline.TimeframeOperator, timeline.TimeframeValues, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		conditions = append(conditions, condition)
		args = append(args, values...)
		argIndex = next
	}
	for _, filter := range timeline.Filters {
		condition, values, next, err := qb.parseTimelineFilter(filter, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		conditions = append(conditions, condition)
		args = append(args, values...)
		argIndex = next
	}
	mhConditions := make([]string, 0, 3)
	if timeline.TemplateID != nil && *timeline.TemplateID != "" {
		args = append(args, *timeline.TemplateID)
		mhConditions = append(mhConditions, fmt.Sprintf("template_id = $%d", argIndex))
		argIndex++
	}
	if timeline.BroadcastID != nil && *timeline.BroadcastID != "" {
		args = append(args, *timeline.BroadcastID)
		mhConditions = append(mhConditions, fmt.Sprintf("broadcast_id = $%d", argIndex))
		argIndex++
	}
	if timeline.LinkURL != nil && *timeline.LinkURL != "" {
		args = append(args, *timeline.LinkURL)
		mhConditions = append(mhConditions, fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_object_keys(CASE WHEN jsonb_typeof(clicked_links) = 'object' THEN clicked_links ELSE '{}'::jsonb END) AS k WHERE strpos(lower(k), lower($%d)) > 0)", argIndex))
		argIndex++
	}
	if len(mhConditions) > 0 {
		conditions = append(conditions, "ct.entity_id IN (SELECT id FROM message_history WHERE "+strings.Join(mhConditions, " AND ")+")")
	}
	comparison := map[string]string{"at_least": ">=", "at_most": "<=", "exactly": "="}[timeline.CountOperator]
	if comparison == "" {
		return "", nil, argIndex, fmt.Errorf("invalid count_operator: %s", timeline.CountOperator)
	}
	args = append(args, timeline.CountValue)
	result := fmt.Sprintf("(SELECT COUNT(*) FROM contact_timeline ct WHERE %s) %s $%d", strings.Join(conditions, " AND "), comparison, argIndex)
	return result, args, argIndex + 1, nil
}

func (qb *QueryBuilder) parseCustomerGoalCondition(goal *domain.CustomEventsGoalCondition, argIndex int) (string, []interface{}, int, error) {
	if goal == nil {
		return "", nil, argIndex, fmt.Errorf("custom_events_goal condition cannot be nil")
	}
	conditions := []string{customerTimelineSubject("ce"), "ce.deleted_at IS NULL"}
	args := make([]interface{}, 0)
	if goal.GoalType == "*" {
		conditions = append(conditions, "ce.goal_type IS NOT NULL")
	} else {
		args = append(args, goal.GoalType)
		conditions = append(conditions, fmt.Sprintf("ce.goal_type = $%d", argIndex))
		argIndex++
	}
	if goal.GoalName != nil && *goal.GoalName != "" {
		args = append(args, *goal.GoalName)
		conditions = append(conditions, fmt.Sprintf("ce.goal_name = $%d", argIndex))
		argIndex++
	}
	if goal.EventName != nil && *goal.EventName != "" {
		args = append(args, *goal.EventName)
		conditions = append(conditions, fmt.Sprintf("ce.event_name = $%d", argIndex))
		argIndex++
	}
	if goal.TimeframeOperator != "" && goal.TimeframeOperator != "anytime" {
		condition, values, next, err := qb.parseGoalTimeframeCondition(goal.TimeframeOperator, goal.TimeframeValues, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		conditions = append(conditions, condition)
		args = append(args, values...)
		argIndex = next
	}
	for _, filter := range goal.Filters {
		condition, values, next, err := qb.parseEventPropertyFilter(filter, argIndex)
		if err != nil {
			return "", nil, argIndex, err
		}
		conditions = append(conditions, condition)
		args = append(args, values...)
		argIndex = next
	}
	aggregate := map[string]string{
		"sum": "COALESCE(SUM(ce.goal_value), 0)", "count": "COUNT(*)", "avg": "COALESCE(AVG(ce.goal_value), 0)",
		"min": "MIN(ce.goal_value)", "max": "MAX(ce.goal_value)",
	}[goal.AggregateOperator]
	if aggregate == "" {
		return "", nil, argIndex, fmt.Errorf("invalid aggregate_operator: %s", goal.AggregateOperator)
	}
	comparison := ""
	switch goal.Operator {
	case "gte", "lte", "eq":
		operator := map[string]string{"gte": ">=", "lte": "<=", "eq": "="}[goal.Operator]
		args = append(args, goal.Value)
		comparison = fmt.Sprintf("%s %s $%d", aggregate, operator, argIndex)
		argIndex++
	case "between":
		if goal.Value2 == nil {
			return "", nil, argIndex, fmt.Errorf("between operator requires value_2")
		}
		args = append(args, goal.Value, *goal.Value2)
		comparison = fmt.Sprintf("%s BETWEEN $%d AND $%d", aggregate, argIndex, argIndex+1)
		argIndex += 2
	default:
		return "", nil, argIndex, fmt.Errorf("invalid operator: %s", goal.Operator)
	}
	result := fmt.Sprintf("EXISTS (SELECT 1 FROM custom_events ce WHERE %s GROUP BY COALESCE(ce.customer_id::text, ce.email) HAVING %s)", strings.Join(conditions, " AND "), comparison)
	if goal.Negate {
		result = "NOT " + result
	}
	return result, args, argIndex, nil
}
