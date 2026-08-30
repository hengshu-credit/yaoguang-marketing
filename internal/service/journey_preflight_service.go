package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const journeyPreflightTTL = 5 * time.Minute

type JourneyPreflightService struct {
	source domain.JourneyPreflightSource
	auth   domain.AuthService
	now    func() time.Time
}

func NewJourneyPreflightService(source domain.JourneyPreflightSource, auth domain.AuthService, now func() time.Time) (*JourneyPreflightService, error) {
	if source == nil {
		return nil, errors.New("journey preflight source is required")
	}
	if now == nil {
		now = time.Now
	}
	return &JourneyPreflightService{source: source, auth: auth, now: now}, nil
}

func (s *JourneyPreflightService) PreflightAutomation(ctx context.Context, request domain.JourneyPreflightRequest) (*domain.JourneyPreflightResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if s.auth != nil {
		var membership *domain.UserWorkspace
		var err error
		ctx, _, membership, err = s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("authenticate journey preflight: %w", err)
		}
		if !membership.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeRead, "Insufficient permissions: read access to automations required")
		}
	}
	snapshot, err := s.source.LoadJourneyPreflightSnapshot(ctx, request.WorkspaceID, request.AutomationID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.Automation == nil {
		return nil, domain.ErrJourneyTraceNotFound
	}
	now := s.now().UTC()
	result := &domain.JourneyPreflightResult{
		WorkspaceID: request.WorkspaceID, AutomationID: request.AutomationID,
		Issues: []domain.JourneyPreflightIssue{}, GeneratedAt: now, ExpiresAt: now.Add(journeyPreflightTTL),
	}
	add := func(code string, severity domain.JourneyPreflightSeverity, title, description, nodeID, fixPath string) {
		result.Issues = append(result.Issues, domain.JourneyPreflightIssue{Code: code, Severity: severity, Title: title, Description: description, NodeID: nodeID, FixPath: fixPath})
		if severity == domain.JourneyPreflightBlocking {
			result.BlockingCount++
		} else {
			result.WarningCount++
		}
	}

	automation := snapshot.Automation
	if automation.Trigger == nil {
		add("trigger_missing", domain.JourneyPreflightBlocking, "未配置触发器", "请选择进入 Journey 的业务事件。", "", "/automations/"+automation.ID)
	} else {
		if !automation.Trigger.Frequency.IsValid() {
			add("trigger_frequency_invalid", domain.JourneyPreflightBlocking, "进入频次无效", "进入频次只能是每个 Customer 一次或每个 Event 一次。", "", "/automations/"+automation.ID)
		}
		if err := automation.Trigger.Validate(); err != nil {
			add("trigger_invalid", domain.JourneyPreflightBlocking, "触发条件不完整", err.Error(), "", "/automations/"+automation.ID)
		}
	}

	edges, graphIssues := journeyGraph(automation)
	for _, issue := range graphIssues {
		add(issue.Code, issue.Severity, issue.Title, issue.Description, issue.NodeID, issue.FixPath)
	}
	if journeyGraphHasCycle(automation.RootNodeID, edges) {
		add("journey_cycle", domain.JourneyPreflightBlocking, "Journey 存在循环", "当前执行器不允许节点形成循环，请断开回路后再激活。", "", "/automations/"+automation.ID)
	}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		if err := validateJourneyNodeConfig(automation, node); err != nil {
			add("node_config_invalid", domain.JourneyPreflightBlocking, "节点配置不完整", err.Error(), node.ID, "/automations/"+automation.ID)
		}
	}

	for _, check := range snapshot.TemplateChecks {
		if !check.Exists {
			add("template_missing", domain.JourneyPreflightBlocking, "模板不可用", "节点引用的模板或版本不存在。", check.NodeID, "/templates")
		} else if !check.ChannelMatches {
			add("template_channel_mismatch", domain.JourneyPreflightBlocking, "模板渠道不匹配", "所选模板渠道与节点渠道不一致。", check.NodeID, "/templates")
		}
		if !check.ProviderReady {
			add("provider_missing", domain.JourneyPreflightBlocking, "触达渠道未配置", "请为该节点配置可用的渠道 Provider。", check.NodeID, "/settings/integrations")
		}
	}
	variableNodeIDs := make([]string, 0, len(snapshot.VariableErrors))
	for nodeID := range snapshot.VariableErrors {
		variableNodeIDs = append(variableNodeIDs, nodeID)
	}
	sort.Strings(variableNodeIDs)
	for _, nodeID := range variableNodeIDs {
		failures := snapshot.VariableErrors[nodeID]
		if len(failures) > 0 {
			add("variable_sample_failed", domain.JourneyPreflightWarning, "变量样例未全部通过", strings.Join(failures, "; "), nodeID, "/templates")
		}
	}
	if len(snapshot.TemplateChecks) > 0 && (!snapshot.HasFrequencyPolicy || len(snapshot.MissingFrequencyChannels) > 0) {
		description := "Journey 的进入频次不替代活动级、触发级和 Workspace 全局消息频控。"
		if len(snapshot.MissingFrequencyChannels) > 0 {
			description += " 缺少渠道：" + strings.Join(snapshot.MissingFrequencyChannels, "、") + "。"
		}
		add("frequency_policy_missing", domain.JourneyPreflightWarning, "尚未完整配置消息频控", description, "", "/settings/frequency")
	}
	for _, node := range automation.Nodes {
		if node != nil && node.Type == domain.NodeTypeWebhook && strings.TrimSpace(configString(node.Config, "secret")) == "" {
			add("webhook_secret_missing", domain.JourneyPreflightWarning, "Webhook 未配置签名密钥", "未签名的 Webhook 更容易被伪造，建议配置密钥。", node.ID, "/automations/"+automation.ID)
		}
	}
	if err := result.Seal(automation.UpdatedAt); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *JourneyPreflightService) ValidateAutomationPreflight(ctx context.Context, request domain.JourneyPreflightRequest, expectedHash string, confirmWarnings bool) error {
	if strings.TrimSpace(expectedHash) == "" {
		return domain.ErrJourneyPreflightRequired
	}
	parts := strings.Split(expectedHash, ".")
	if len(parts) != 2 {
		return domain.ErrJourneyPreflightChanged
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || s.now().UTC().After(time.Unix(expiresUnix, 0).UTC()) {
		return domain.ErrJourneyPreflightChanged
	}
	result, err := s.PreflightAutomation(ctx, request)
	if err != nil {
		return err
	}
	result.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	snapshot, err := s.source.LoadJourneyPreflightSnapshot(ctx, request.WorkspaceID, request.AutomationID)
	if err != nil {
		return err
	}
	if err := result.Seal(snapshot.Automation.UpdatedAt); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(result.SummaryHash), []byte(expectedHash)) != 1 {
		return domain.ErrJourneyPreflightChanged
	}
	if result.BlockingCount > 0 {
		return domain.ErrJourneyPreflightBlocked
	}
	if result.WarningCount > 0 && !confirmWarnings {
		return domain.ErrJourneyPreflightWarningConfirmation
	}
	return nil
}

func journeyGraph(automation *domain.Automation) (map[string][]string, []domain.JourneyPreflightIssue) {
	edges := make(map[string][]string, len(automation.Nodes))
	nodes := make(map[string]struct{}, len(automation.Nodes))
	for _, node := range automation.Nodes {
		if node != nil {
			nodes[node.ID] = struct{}{}
		}
	}
	issues := []domain.JourneyPreflightIssue{}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		targets := journeyNodeTargets(node)
		for _, target := range targets {
			if _, ok := nodes[target]; !ok {
				issues = append(issues, domain.JourneyPreflightIssue{Code: "node_target_missing", Severity: domain.JourneyPreflightBlocking, Title: "节点连接已失效", Description: "节点指向不存在的下一步：" + target, NodeID: node.ID, FixPath: "/automations/" + automation.ID})
				continue
			}
			edges[node.ID] = append(edges[node.ID], target)
		}
	}
	reachable := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		for _, next := range edges[id] {
			visit(next)
		}
	}
	if automation.RootNodeID != "" {
		if _, ok := nodes[automation.RootNodeID]; !ok {
			issues = append(issues, domain.JourneyPreflightIssue{Code: "root_node_missing", Severity: domain.JourneyPreflightBlocking, Title: "起始节点不存在", Description: "请选择有效的 Journey 起始节点。", FixPath: "/automations/" + automation.ID})
		} else {
			visit(automation.RootNodeID)
		}
	} else {
		issues = append(issues, domain.JourneyPreflightIssue{Code: "root_node_missing", Severity: domain.JourneyPreflightBlocking, Title: "未设置起始节点", Description: "请选择 Journey 起始节点。", FixPath: "/automations/" + automation.ID})
	}
	nodeIDs := make([]string, 0, len(nodes))
	for id := range nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, id := range nodeIDs {
		if !reachable[id] {
			issues = append(issues, domain.JourneyPreflightIssue{Code: "node_unreachable", Severity: domain.JourneyPreflightBlocking, Title: "存在不可达节点", Description: "该节点不会从起始节点被执行，请连接或删除。", NodeID: id, FixPath: "/automations/" + automation.ID})
		}
	}
	return edges, issues
}

func journeyNodeTargets(node *domain.AutomationNode) []string {
	targets := []string{}
	if node.NextNodeID != nil && strings.TrimSpace(*node.NextNodeID) != "" {
		targets = append(targets, strings.TrimSpace(*node.NextNodeID))
	}
	for _, key := range []string{"continue_node_id", "exit_node_id", "not_in_list_node_id", "active_node_id", "non_active_node_id"} {
		if value := configString(node.Config, key); value != "" {
			targets = append(targets, value)
		}
	}
	for _, key := range []string{"paths", "variants"} {
		items, _ := node.Config[key].([]interface{})
		for _, item := range items {
			if object, ok := item.(map[string]interface{}); ok {
				if value := configString(object, "next_node_id"); value != "" {
					targets = append(targets, value)
				}
			}
		}
	}
	return targets
}

func validateJourneyNodeConfig(automation *domain.Automation, node *domain.AutomationNode) error {
	if err := node.ValidateForAutomation(automation); err != nil {
		return err
	}
	decode := func(target interface{}) error {
		payload, err := json.Marshal(node.Config)
		if err != nil {
			return err
		}
		return json.Unmarshal(payload, target)
	}
	switch node.Type {
	case domain.NodeTypeTrigger:
		return nil
	case domain.NodeTypeDelay:
		var config domain.DelayNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeEmail:
		var config domain.EmailNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeSMS, domain.NodeTypePush:
		var config domain.ChannelNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeAddToList:
		var config domain.AddToListNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeRemoveFromList:
		var config domain.RemoveFromListNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeListStatusBranch:
		var config domain.ListStatusBranchNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeABTest:
		var config domain.ABTestNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeWebhook:
		var config domain.WebhookNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		return config.Validate()
	case domain.NodeTypeBranch:
		var config domain.BranchNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		for _, path := range config.Paths {
			if path.Conditions != nil {
				if err := path.Conditions.Validate(); err != nil {
					return fmt.Errorf("branch %s conditions: %w", path.Name, err)
				}
			}
		}
	case domain.NodeTypeFilter:
		var config domain.FilterNodeConfig
		if err := decode(&config); err != nil {
			return err
		}
		if config.Conditions == nil {
			return errors.New("filter conditions are required")
		}
		return config.Conditions.Validate()
	}
	return nil
}

func journeyGraphHasCycle(root string, edges map[string][]string) bool {
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, next := range edges[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	return root != "" && visit(root)
}

func configString(config map[string]interface{}, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

type AutomationJourneyPreflightSource struct {
	automations domain.AutomationRepository
	workspaces  domain.WorkspaceRepository
	templates   domain.TemplateService
}

func NewAutomationJourneyPreflightSource(automations domain.AutomationRepository, workspaces domain.WorkspaceRepository, templates domain.TemplateService) (*AutomationJourneyPreflightSource, error) {
	if automations == nil || workspaces == nil || templates == nil {
		return nil, errors.New("automation, workspace and template dependencies are required")
	}
	return &AutomationJourneyPreflightSource{automations: automations, workspaces: workspaces, templates: templates}, nil
}

func (s *AutomationJourneyPreflightSource) LoadJourneyPreflightSnapshot(ctx context.Context, workspaceID, automationID string) (*domain.JourneyPreflightSnapshot, error) {
	automation, err := s.automations.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	snapshot := &domain.JourneyPreflightSnapshot{Automation: automation, VariableErrors: map[string][]string{}, HasFrequencyPolicy: true}
	channels := map[string]struct{}{}
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		channel := ""
		switch node.Type {
		case domain.NodeTypeEmail:
			channel = "email"
		case domain.NodeTypeSMS:
			channel = "sms"
		case domain.NodeTypePush:
			channel = "push"
		}
		if channel == "" {
			continue
		}
		channels[channel] = struct{}{}
		templateID := configString(node.Config, "template_id")
		version := configInt64(node.Config, "template_version")
		check := domain.JourneyTemplateCheck{NodeID: node.ID, Channel: channel, TemplateID: templateID, TemplateVersion: version}
		if templateID != "" {
			template, templateErr := s.templates.GetTemplateByID(ctx, workspaceID, templateID, version)
			if templateErr == nil && template != nil {
				check.Exists = true
				check.TemplateVersion = template.Version
				check.ChannelMatches = strings.EqualFold(template.Channel, channel)
			}
		}
		integrationID := configString(node.Config, "integration_id")
		for _, integration := range workspace.Integrations {
			if integrationID != "" && integration.ID != integrationID {
				continue
			}
			if string(integration.Type) == channel {
				check.ProviderReady = true
				break
			}
		}
		snapshot.TemplateChecks = append(snapshot.TemplateChecks, check)
		if raw, ok := node.Config["variable_errors"]; ok {
			snapshot.VariableErrors[node.ID] = stringSlice(raw)
		}
	}
	if len(channels) > 0 {
		db, dbErr := s.workspaces.GetConnection(ctx, workspaceID)
		if dbErr != nil {
			return nil, dbErr
		}
		for channel := range channels {
			var exists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM frequency_policies
				WHERE enabled AND channel = $2
					AND (scope = 'workspace_global' OR (scope = 'trigger' AND scope_ref = $1))
			)`, automationID, channel).Scan(&exists); err != nil {
				return nil, fmt.Errorf("check journey frequency policy for %s: %w", channel, err)
			}
			if !exists {
				snapshot.MissingFrequencyChannels = append(snapshot.MissingFrequencyChannels, channel)
			}
		}
		sort.Strings(snapshot.MissingFrequencyChannels)
		snapshot.HasFrequencyPolicy = len(snapshot.MissingFrequencyChannels) == 0
	}
	return snapshot, nil
}

func configInt64(config map[string]interface{}, key string) int64 {
	switch value := config[key].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	case json.Number:
		result, _ := value.Int64()
		return result
	default:
		return 0
	}
}

func stringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{strings.TrimSpace(typed)}
		}
	}
	return nil
}
