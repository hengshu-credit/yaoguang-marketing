package schema

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type businessSchemaExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// MigrateBusinessAttributeLabels preserves workspace-specific legacy field names
// as ordinary editable definitions; new attributes have no five-slot restriction.
func MigrateBusinessAttributeLabels(ctx context.Context, db businessSchemaExecutor, labels map[string]string) error {
	for key, name := range labels {
		typ := "string"
		if strings.HasPrefix(key, "custom_number_") {
			typ = "number"
		}
		if strings.HasPrefix(key, "custom_json_") {
			typ = "object"
		}
		schema := map[string]any{"type": []any{typ, "null"}}
		if strings.HasPrefix(key, "custom_datetime_") {
			schema["format"] = "date-time"
		}
		encoded, err := json.Marshal(schema)
		if err != nil {
			return err
		}
		settings, _ := json.Marshal(map[string]any{"legacy_field": key, "writable": true})
		if _, err = db.ExecContext(ctx, `INSERT INTO business_definitions(id,kind,key,name,schema,settings,status) VALUES(md5('business:legacy:'||$1)::uuid,'customer_attribute',$1,$2,$3,$4,'active') ON CONFLICT(kind,key) DO NOTHING`, key, name, encoded, settings); err != nil {
			return fmt.Errorf("migrate legacy attribute label: %w", err)
		}
	}
	_, err := db.ExecContext(ctx, `INSERT INTO business_definition_revisions(definition_id,revision,document) SELECT id,revision,to_jsonb(d) FROM business_definitions d WHERE settings ? 'legacy_field' ON CONFLICT(definition_id,revision) DO NOTHING`)
	return err
}
