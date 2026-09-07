package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *ExecutionSnapshotPostgresRepository) SaveRuntimeNodes(ctx context.Context, ws, id string, raw json.RawMessage, encrypted string, providers bool) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO execution_runtime_credentials(id,automation_id,configuration_hash,encrypted_nodes,includes_providers) VALUES(gen_random_uuid(),$1,encode(sha256(convert_to($2::jsonb::text,'UTF8')),'hex'),$3,$4)`, id, []byte(raw), encrypted, providers)
	return err
}
func (r *ExecutionSnapshotPostgresRepository) RuntimeNodes(ctx context.Context, ws, id string) (string, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return "", err
	}
	var encrypted string
	err = db.QueryRowContext(ctx, `SELECT encrypted_nodes FROM execution_runtime_credentials WHERE id=$1::uuid`, id).Scan(&encrypted)
	return encrypted, err
}
func (r *ExecutionSnapshotPostgresRepository) UnarchivedRuntimeAutomations(ctx context.Context, ws string, limit int) ([]*domain.Automation, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT to_jsonb(a) FROM automations a CROSS JOIN LATERAL(SELECT jsonb_object_agg(n->>'id',n->'config') configs FROM jsonb_array_elements(a.nodes)n)x WHERE a.status='live' AND (x.configs IS DISTINCT FROM notifuse_redact_execution_json(x.configs) OR EXISTS(SELECT 1 FROM jsonb_array_elements(a.nodes)n WHERE n->>'type'='email' OR COALESCE(n->'config'->>'integration_id','')<>'')) AND NOT EXISTS(SELECT 1 FROM execution_runtime_credentials r WHERE r.automation_id=a.id AND r.configuration_hash=encode(sha256(convert_to(x.configs::text,'UTF8')),'hex') AND (r.includes_providers OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements(a.nodes)n WHERE n->>'type'='email' OR COALESCE(n->'config'->>'integration_id','')<>''))) LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.Automation{}
	for rows.Next() {
		var raw []byte
		var a domain.Automation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ProviderAutomationsPage selects every active configuration in stable bounded pages.
func (r *ExecutionSnapshotPostgresRepository) ProviderAutomationsPage(ctx context.Context, ws, after string, limit int) ([]*domain.Automation, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT to_jsonb(a) FROM automations a WHERE status IN('live','paused') AND id>$1 AND EXISTS(SELECT 1 FROM jsonb_array_elements(a.nodes)n WHERE n->>'type'='email' OR COALESCE(n->'config'->>'integration_id','')<>'') ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*domain.Automation{}
	for rows.Next() {
		var raw []byte
		var a domain.Automation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		result = append(result, &a)
	}
	return result, rows.Err()
}

func (r *ExecutionSnapshotPostgresRepository) WithProviderRefreshLock(ctx context.Context, ws string, apply func() error) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('execution-provider-refresh'))`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext('execution-provider-refresh'))`)
	return apply()
}
func (r *ExecutionSnapshotPostgresRepository) SetProviderRefreshPending(ctx context.Context, ws string, pending bool) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO execution_provider_refresh(singleton,pending) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET pending=EXCLUDED.pending`, pending)
	return err
}
func (r *ExecutionSnapshotPostgresRepository) ProviderRefreshPending(ctx context.Context, ws string) (bool, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return false, err
	}
	var pending bool
	err = db.QueryRowContext(ctx, `SELECT COALESCE((SELECT pending FROM execution_provider_refresh WHERE singleton),false)`).Scan(&pending)
	return pending, err
}

func (r *ExecutionSnapshotPostgresRepository) SnapshotForChannelSend(ctx context.Context, ws, integrationID, messageID, effectKey, providerMessageID string) (*domain.ExecutionSnapshot, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	var executionID string
	err = db.QueryRowContext(ctx, `SELECT e.execution_id FROM channel_send_executions c JOIN execution_evidence e ON e.delivery_id=c.message_id AND e.kind='delivery_content' WHERE c.integration_id=$1 AND ($2<>'' OR $3<>'' OR $4<>'') AND ($2='' OR c.message_id=$2) AND ($3='' OR c.effect_key=$3) AND ($4='' OR c.provider_message_id=$4 OR (c.provider_message_id IS NULL AND ($2<>'' OR $3<>''))) ORDER BY e.occurred_at LIMIT 1`, integrationID, messageID, effectKey, providerMessageID).Scan(&executionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrExecutionSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, ws, executionID)
}

func (r *ExecutionSnapshotPostgresRepository) LatestRuntimeReference(ctx context.Context, ws, id string) (string, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return "", err
	}
	var reference string
	err = db.QueryRowContext(ctx, `SELECT id::text FROM execution_runtime_credentials WHERE automation_id=$1 AND includes_providers ORDER BY created_at DESC,id DESC LIMIT 1`, id).Scan(&reference)
	return reference, err
}
