package repository

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func (r *ExecutionSnapshotPostgresRepository) UnarchivedAutomationTemplates(ctx context.Context, ws string, limit int) ([]*domain.Template, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT to_jsonb(t) FROM automations a CROSS JOIN LATERAL jsonb_array_elements(a.nodes) n JOIN LATERAL(SELECT x.* FROM templates x WHERE x.id=n->'config'->>'template_id' AND x.deleted_at IS NULL AND (COALESCE((n->'config'->>'template_version')::bigint,0)=0 OR x.version=(n->'config'->>'template_version')::bigint) ORDER BY x.version DESC LIMIT 1)t ON true WHERE a.status='live' AND NOT EXISTS(SELECT 1 FROM execution_template_material_archives m WHERE m.template_id=t.id AND m.content=notifuse_redact_execution_json(jsonb_build_object('channel',t.channel,'email',t.email,'web',t.web,'sms',t.sms,'push',t.push,'content',t.content,'translations',COALESCE(t.translations,'{}'::jsonb),'settings',t.settings,'template_macro_id',t.template_macro_id))) LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.Template{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var template domain.Template
		if err = json.Unmarshal(raw, &template); err != nil {
			return nil, err
		}
		out = append(out, &template)
	}
	return out, rows.Err()
}
