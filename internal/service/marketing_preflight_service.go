package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

const marketingPreflightTTL = 5 * time.Minute

type MarketingPreflightService struct {
	source domain.MarketingPreflightSource
	auth   domain.AuthService
	now    func() time.Time
}

func NewMarketingPreflightService(source domain.MarketingPreflightSource, auth domain.AuthService, now func() time.Time) (*MarketingPreflightService, error) {
	if source == nil {
		return nil, errors.New("marketing preflight source is required")
	}
	if now == nil {
		now = time.Now
	}
	return &MarketingPreflightService{source: source, auth: auth, now: now}, nil
}

func (s *MarketingPreflightService) PreflightBroadcast(ctx context.Context, request domain.MarketingPreflightRequest) (*domain.MarketingPreflightResult, error) {
	if strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.BroadcastID) == "" {
		return nil, errors.New("workspace_id and broadcast_id are required")
	}
	if s.auth != nil {
		var membership *domain.UserWorkspace
		var err error
		ctx, _, membership, err = s.auth.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("authenticate preflight: %w", err)
		}
		if !membership.HasPermission(domain.PermissionResourceBroadcasts, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(domain.PermissionResourceBroadcasts, domain.PermissionTypeRead, "Broadcast read access is required for preflight")
		}
	}
	snapshot, err := s.source.LoadMarketingPreflightSnapshot(ctx, request.WorkspaceID, request.BroadcastID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := &domain.MarketingPreflightResult{
		WorkspaceID: request.WorkspaceID, BroadcastID: request.BroadcastID, Counts: snapshot.Counts,
		Issues: []domain.MarketingPreflightIssue{}, GeneratedAt: now, ExpiresAt: time.Unix(now.Add(marketingPreflightTTL).Unix(), 0).UTC(),
	}
	add := func(code string, severity domain.MarketingPreflightSeverity, title, description, fixPath string) {
		result.Issues = append(result.Issues, domain.MarketingPreflightIssue{Code: code, Severity: severity, Title: title, Description: description, FixPath: fixPath})
		if severity == domain.MarketingPreflightBlocking {
			result.BlockingCount++
		} else {
			result.WarningCount++
		}
	}
	if snapshot.Counts.TargetTotal == 0 {
		add("audience_empty", domain.MarketingPreflightBlocking, "没有可发送的目标客户", "当前活动客群为空，请检查客群或名单条件。", "/audiences")
	}
	if !snapshot.HasProvider {
		add("provider_missing", domain.MarketingPreflightBlocking, "尚未配置营销渠道", "请先配置营销 Email Provider，再开始发送。", "/settings/integrations")
	}
	if len(snapshot.MissingTemplates) > 0 {
		add("template_missing", domain.MarketingPreflightBlocking, "活动模板不可用", "一个或多个活动版本缺少有效模板。", "/templates")
	}
	if len(snapshot.TemplateChannelMismatch) > 0 {
		add("template_channel_mismatch", domain.MarketingPreflightBlocking, "模板渠道不匹配", "活动渠道和所选模板渠道不一致。", "/templates")
	}
	if snapshot.Counts.MissingIdentity > 0 {
		add("identity_missing", domain.MarketingPreflightWarning, "部分客户缺少有效身份标识", fmt.Sprintf("%d 位客户没有启用的渠道身份，将不会发送。", snapshot.Counts.MissingIdentity), "/customers")
	}
	if snapshot.Counts.MissingConsent > 0 {
		add("consent_missing", domain.MarketingPreflightWarning, "部分客户缺少营销同意", fmt.Sprintf("%d 位客户没有有效营销同意，请确认合规策略。", snapshot.Counts.MissingConsent), "/customers")
	}
	if snapshot.Counts.Suppressed > 0 {
		add("recipient_suppressed", domain.MarketingPreflightWarning, "部分客户已被触达抑制", fmt.Sprintf("%d 位客户已退订、退信或投诉，将不会发送。", snapshot.Counts.Suppressed), "/delivery")
	}
	if snapshot.Counts.FrequencyDeny > 0 {
		add("frequency_expected_deny", domain.MarketingPreflightWarning, "部分客户预计命中频控", fmt.Sprintf("预计 %d 位客户本次不会触达。实际结果以发送时原子频控为准。", snapshot.Counts.FrequencyDeny), "/settings/frequency")
	}
	if snapshot.Counts.VariableFailures > 0 {
		add("variable_sample_failed", domain.MarketingPreflightWarning, "变量样例未全部通过", fmt.Sprintf("%d 个有界样例无法完整渲染，请检查模板默认值。", snapshot.Counts.VariableFailures), "/templates")
	}
	if snapshot.AudienceBuildStale {
		add("audience_build_stale", domain.MarketingPreflightWarning, "客群构建不是最新版本", "活动开始会使用当前已完成构建；请重新生成客群以包含最新定义。", "/audiences")
	}
	if !snapshot.HasFrequencyPolicy {
		add("frequency_policy_missing", domain.MarketingPreflightWarning, "尚未配置触达频控", "本活动仅使用默认安全策略；建议配置活动级和全局频控。", "/settings/frequency")
	}
	if err := result.Seal(snapshot.BroadcastUpdatedAt); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *MarketingPreflightService) ValidateBroadcastPreflight(ctx context.Context, request domain.MarketingPreflightRequest, expectedHash string) error {
	if strings.TrimSpace(expectedHash) == "" {
		return domain.ErrMarketingPreflightRequired
	}
	parts := strings.Split(expectedHash, ".")
	if len(parts) != 2 {
		return domain.ErrMarketingPreflightChanged
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || s.now().UTC().After(time.Unix(expiresUnix, 0).UTC()) {
		return domain.ErrMarketingPreflightChanged
	}
	result, err := s.PreflightBroadcast(ctx, request)
	if err != nil {
		return err
	}
	result.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	// Seal needs the persisted broadcast update time rather than GeneratedAt.
	snapshot, err := s.source.LoadMarketingPreflightSnapshot(ctx, request.WorkspaceID, request.BroadcastID)
	if err != nil {
		return err
	}
	if err := result.Seal(snapshot.BroadcastUpdatedAt); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(result.SummaryHash), []byte(expectedHash)) != 1 {
		return domain.ErrMarketingPreflightChanged
	}
	if result.BlockingCount > 0 {
		return domain.ErrMarketingPreflightBlocked
	}
	return nil
}

// BroadcastMarketingPreflightSource reads only aggregate recipient counts and
// bounded configuration metadata. It never loads a large audience into memory.
type BroadcastMarketingPreflightSource struct {
	broadcasts domain.BroadcastRepository
	workspaces domain.WorkspaceRepository
	templates  domain.TemplateService
}

func NewBroadcastMarketingPreflightSource(broadcasts domain.BroadcastRepository, workspaces domain.WorkspaceRepository, templates domain.TemplateService) (*BroadcastMarketingPreflightSource, error) {
	if broadcasts == nil || workspaces == nil || templates == nil {
		return nil, errors.New("broadcast, workspace and template dependencies are required")
	}
	return &BroadcastMarketingPreflightSource{broadcasts: broadcasts, workspaces: workspaces, templates: templates}, nil
}

func (s *BroadcastMarketingPreflightSource) LoadMarketingPreflightSnapshot(ctx context.Context, workspaceID, broadcastID string) (*domain.MarketingPreflightSnapshot, error) {
	broadcast, err := s.broadcasts.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	provider, err := workspace.GetEmailProvider(true)
	if err != nil {
		return nil, err
	}
	snapshot := &domain.MarketingPreflightSnapshot{
		WorkspaceID: workspaceID, BroadcastID: broadcastID, BroadcastUpdatedAt: broadcast.UpdatedAt,
		HasProvider: provider != nil, MissingTemplates: []string{}, TemplateChannelMismatch: []string{},
	}
	seenTemplates := map[string]struct{}{}
	for _, variation := range broadcast.TestSettings.Variations {
		if strings.TrimSpace(variation.TemplateID) == "" {
			snapshot.MissingTemplates = append(snapshot.MissingTemplates, "")
			continue
		}
		if _, seen := seenTemplates[variation.TemplateID]; seen {
			continue
		}
		seenTemplates[variation.TemplateID] = struct{}{}
		template, templateErr := s.templates.GetTemplateByID(ctx, workspaceID, variation.TemplateID, 0)
		if templateErr != nil || template == nil || template.DeletedAt != nil {
			snapshot.MissingTemplates = append(snapshot.MissingTemplates, variation.TemplateID)
			continue
		}
		if template.Channel != "" && broadcast.ChannelType != "" && template.Channel != broadcast.ChannelType {
			snapshot.TemplateChannelMismatch = append(snapshot.TemplateChannelMismatch, variation.TemplateID)
		}
	}
	if len(broadcast.TestSettings.Variations) == 0 {
		snapshot.MissingTemplates = append(snapshot.MissingTemplates, "")
	}
	db, err := s.workspaces.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := loadMarketingRecipientCounts(ctx, db, broadcast.Audience, broadcast.ChannelType, &snapshot.Counts); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM frequency_policies WHERE enabled = TRUE AND (
			(scope = 'campaign' AND scope_ref = $1) OR scope = 'workspace_global'
		) AND (channel = $2 OR channel = '*')
	)`, broadcastID, broadcast.ChannelType).Scan(&snapshot.HasFrequencyPolicy); err != nil && !isUndefinedTable(err) {
		return nil, err
	}
	audienceID := strings.TrimSpace(broadcast.Audience.AudienceID)
	if audienceID == "" {
		audienceID, _ = stringMetadata(broadcast.Metadata, "audience_id")
	}
	if audienceID != "" {
		var stale bool
		err = db.QueryRowContext(ctx, `SELECT active_build_id IS NULL OR active_version <> COALESCE((
			SELECT audience_version FROM audience_builds WHERE id = active_build_id AND status = 'completed'
		), 0) FROM audiences WHERE id = NULLIF($1, '')::uuid`, audienceID).Scan(&stale)
		if err == nil {
			snapshot.AudienceBuildStale = stale
		} else if !errors.Is(err, sql.ErrNoRows) && !isUndefinedTable(err) {
			return nil, err
		}
	}
	return snapshot, nil
}

func loadMarketingRecipientCounts(ctx context.Context, db *sql.DB, audience domain.AudienceSettings, channel string, counts *domain.MarketingPreflightCounts) error {
	if strings.TrimSpace(audience.AudienceID) != "" {
		identityType := channel
		if identityType == "" {
			identityType = "email"
		}
		return db.QueryRowContext(ctx, `WITH source_build AS (
			SELECT build.id FROM audience_builds build
			WHERE build.audience_id = NULLIF($1, '')::uuid AND build.audience_version = $2
				AND build.status = 'completed'
				AND ($3 = '' OR build.id = NULLIF($3, '')::uuid)
			ORDER BY build.completed_at DESC, build.id DESC LIMIT 1
		), classified AS (
			SELECT membership.customer_id,
				EXISTS (SELECT 1 FROM contact_lists legacy_list WHERE legacy_list.customer_id = membership.customer_id
					AND legacy_list.status IN ('unsubscribed', 'bounced', 'complained')) AS suppressed,
				EXISTS (SELECT 1 FROM customer_identities identity
					WHERE identity.customer_id = membership.customer_id AND identity.identity_type = $4
					AND identity.enabled = TRUE) AS has_identity,
				EXISTS (SELECT 1 FROM customer_consents consent
					WHERE consent.customer_id = membership.customer_id AND consent.channel = $4
					AND consent.status IN ('granted', 'subscribed', 'opted_in', 'active')
					AND consent.revoked_at IS NULL AND consent.valid_from <= CURRENT_TIMESTAMP) AS has_consent
			FROM audience_memberships membership JOIN source_build ON source_build.id = membership.build_id
		) SELECT COUNT(*),
			COUNT(*) FILTER (WHERE has_identity AND has_consent AND NOT suppressed),
			COUNT(*) FILTER (WHERE NOT has_identity), COUNT(*) FILTER (WHERE NOT has_consent),
			COUNT(*) FILTER (WHERE suppressed) FROM classified`, audience.AudienceID, audience.AudienceVersion,
			audience.AudienceBuildID, identityType).Scan(&counts.TargetTotal, &counts.Reachable,
			&counts.MissingIdentity, &counts.MissingConsent, &counts.Suppressed)
	}
	listID := audience.List
	if strings.TrimSpace(listID) == "" {
		return nil
	}
	identityType := channel
	if identityType == "" {
		identityType = "email"
	}
	return db.QueryRowContext(ctx, `WITH classified AS (
		SELECT membership.customer_id,
			membership.status IN ('unsubscribed', 'bounced', 'complained') AS suppressed,
			EXISTS (SELECT 1 FROM customer_identities identity
				WHERE identity.customer_id = membership.customer_id AND identity.identity_type = $2
				AND identity.enabled = TRUE) AS has_identity,
			EXISTS (SELECT 1 FROM customer_consents consent
				WHERE consent.customer_id = membership.customer_id AND consent.channel = $2
				AND consent.status IN ('granted', 'subscribed', 'opted_in', 'active')
				AND consent.revoked_at IS NULL AND consent.valid_from <= CURRENT_TIMESTAMP) AS has_consent
		FROM customer_list_memberships membership WHERE membership.list_id = $1
	)
	SELECT COUNT(*),
		COUNT(*) FILTER (WHERE has_identity AND has_consent AND NOT suppressed),
		COUNT(*) FILTER (WHERE NOT has_identity),
		COUNT(*) FILTER (WHERE NOT has_consent),
		COUNT(*) FILTER (WHERE suppressed)
	FROM classified`, listID, identityType).Scan(&counts.TargetTotal, &counts.Reachable, &counts.MissingIdentity, &counts.MissingConsent, &counts.Suppressed)
}

func stringMetadata(metadata domain.MapOfAny, key string) (string, bool) {
	if metadata == nil {
		return "", false
	}
	value, ok := metadata[key].(string)
	return strings.TrimSpace(value), ok && strings.TrimSpace(value) != ""
}

func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "does not exist")
}
