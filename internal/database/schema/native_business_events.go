package schema

import (
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
)

func nativeBusinessEventSeed() string {
	values := make([]string, 0, 36)
	for _, event := range domain.NativeBusinessEvents() {
		values = append(values, fmt.Sprintf("('%s','%s')", strings.ReplaceAll(event.Key, "'", "''"), strings.ReplaceAll(event.Name, "'", "''")))
	}
	return `INSERT INTO business_definitions AS d(id,kind,key,name,schema,settings,status) SELECT md5('business:event:'||v.code)::uuid,'event',v.code,v.name,'{"type":"object"}'::jsonb,'{"builtin":true,"subject_type":"customer","expiry_mode":"never","browser_enabled":false}'::jsonb,'active' FROM (VALUES ` + strings.Join(values, ",") + `) AS v(code,name) ON CONFLICT(kind,key) DO UPDATE SET name=EXCLUDED.name,schema=EXCLUDED.schema,settings=EXCLUDED.settings,status=EXCLUDED.status,revision=d.revision+1,updated_at=NOW() WHERE (d.name,d.schema,d.settings,d.status) IS DISTINCT FROM (EXCLUDED.name,EXCLUDED.schema,EXCLUDED.settings,EXCLUDED.status)`
}
