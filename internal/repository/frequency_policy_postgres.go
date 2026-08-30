package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
)

type FrequencyPolicyPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewFrequencyPolicyRepository(workspaceRepo domain.WorkspaceRepository) *FrequencyPolicyPostgresRepository {
	return &FrequencyPolicyPostgresRepository{workspaceRepo: workspaceRepo}
}

func NewFrequencyPolicyRepositoryWithDB(db *sql.DB) *FrequencyPolicyPostgresRepository {
	return &FrequencyPolicyPostgresRepository{db: db}
}

func (r *FrequencyPolicyPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

func (r *FrequencyPolicyPostgresRepository) SaveFrequencyPolicy(ctx context.Context, workspaceID string, policy domain.FrequencyPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO frequency_policies (
		id, version, name, scope, scope_ref, channel, max_events, window_kind,
		window_seconds, timezone, deny_action, priority, enabled, created_at
	) VALUES (NULLIF($1, '')::uuid, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, NULLIF($10, ''), $11, $12, $13, $14)
	ON CONFLICT (id, version) DO NOTHING`, policy.ID, policy.Version, policy.Name, policy.Scope, policy.ScopeRef,
		policy.Channel, policy.MaxEvents, policy.WindowKind, policy.WindowSeconds, policy.Timezone,
		policy.DenyAction, policy.Priority, policy.Enabled, policy.CreatedAt)
	if err != nil {
		return fmt.Errorf("save frequency policy: %w", err)
	}
	return nil
}

// ResolveFrequencyPolicies returns independent campaign, trigger and global
// policies in priority order. Empty campaign/trigger refs simply omit that scope.
func (r *FrequencyPolicyPostgresRepository) ResolveFrequencyPolicies(ctx context.Context, workspaceID, campaignRef, triggerRef, channel string) ([]domain.FrequencyPolicy, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT ON (id) id, version, name, scope, COALESCE(scope_ref, ''), channel,
		max_events, window_kind, window_seconds, COALESCE(timezone, ''), deny_action, priority, enabled, created_at
	FROM frequency_policies
	WHERE enabled = TRUE AND channel = $1 AND (
		scope = 'workspace_global' OR (scope = 'campaign' AND scope_ref = NULLIF($2, '')) OR (scope = 'trigger' AND scope_ref = NULLIF($3, ''))
	)
	ORDER BY id, version DESC`, channel, campaignRef, triggerRef)
	if err != nil {
		return nil, fmt.Errorf("resolve frequency policies: %w", err)
	}
	defer rows.Close()
	policies := []domain.FrequencyPolicy{}
	for rows.Next() {
		var policy domain.FrequencyPolicy
		if err := rows.Scan(&policy.ID, &policy.Version, &policy.Name, &policy.Scope, &policy.ScopeRef, &policy.Channel,
			&policy.MaxEvents, &policy.WindowKind, &policy.WindowSeconds, &policy.Timezone, &policy.DenyAction,
			&policy.Priority, &policy.Enabled, &policy.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan frequency policy: %w", err)
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate frequency policies: %w", err)
	}
	return policies, nil
}

func (r *FrequencyPolicyPostgresRepository) SaveFrequencyDecision(ctx context.Context, workspaceID string, decision domain.FrequencyDecision) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	policies, err := json.Marshal(decision.PolicyIDs)
	if err != nil {
		return err
	}
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = time.Now().UTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO frequency_decisions (
		id, reservation_id, effect_key, customer_id, channel, allowed, deferred, matched_scope, policy_versions, reason, decided_at
	) VALUES (NULLIF($1, '')::uuid, $2, $3, NULLIF($4, '')::uuid, $5, $6, $7, NULLIF($8, ''), $9, NULLIF($10, ''), $11)
	ON CONFLICT (reservation_id) DO NOTHING`, decision.ID, decision.ReservationID, decision.EffectKey,
		decision.CustomerID, decision.Channel, decision.Allowed, decision.Deferred, decision.MatchedScope,
		policies, decision.Reason, decision.DecidedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil
		}
		return fmt.Errorf("save frequency decision: %w", err)
	}
	return nil
}
