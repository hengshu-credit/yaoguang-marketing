package repository

import (
	"context"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *FunnelPostgresRepository) ObjectRelations(ctx context.Context, w string, start, end, watermark time.Time) ([]domain.AnalysisObjectRelation, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `WITH RECURSIVE latest AS (SELECT DISTINCT ON(relation_id) * FROM subject_relation_history WHERE recorded_at<=$3 ORDER BY relation_id,recorded_at DESC,id DESC), aliases AS (SELECT DISTINCT h.from_id origin,c.id::text canonical,c.merged_into_id next,ARRAY[c.id] path FROM latest h JOIN customers c ON c.id=h.from_id::uuid WHERE h.relation_key='__customer_ownership' UNION ALL SELECT a.origin,c.id::text,c.merged_into_id,a.path||c.id FROM aliases a JOIN customers c ON c.id=a.next WHERE NOT c.id=ANY(a.path)), canonical_owners AS (SELECT origin,canonical FROM aliases WHERE next IS NULL), definitions AS (SELECT DISTINCT ON(definition_id) document FROM business_definition_revisions WHERE created_at<=$3 ORDER BY definition_id,revision DESC), edges AS (SELECT relation_key,from_type,from_id,to_type,to_id,valid_from,valid_to FROM latest WHERE relation_key<>'__customer_ownership' UNION ALL SELECT d.document->>'key',h.from_type,COALESCE(owner.canonical,h.from_id),h.to_type,h.to_id,h.valid_from,h.valid_to FROM latest h LEFT JOIN canonical_owners owner ON owner.origin=h.from_id JOIN definitions d ON d.document->>'kind'='object_relation' AND d.document->>'status'='active' AND d.document->'settings'->>'from_type'='customer' AND d.document->'settings'->>'to_type'=h.to_type WHERE h.relation_key='__customer_ownership') SELECT relation_key,from_type,from_id,to_type,to_id,valid_from,valid_to FROM edges WHERE valid_from<=$2 AND (valid_to IS NULL OR valid_to>$1) ORDER BY relation_key,from_id,to_id LIMIT 100001`, start, end, watermark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnalysisObjectRelation{}
	for rows.Next() {
		var relation domain.AnalysisObjectRelation
		if err = rows.Scan(&relation.Key, &relation.FromType, &relation.FromID, &relation.ToType, &relation.ToID, &relation.ValidFrom, &relation.ValidTo); err != nil {
			return nil, err
		}
		out = append(out, relation)
	}
	if len(out) > 100000 {
		return nil, fmt.Errorf("object relation query limit exceeded")
	}
	return out, rows.Err()
}
func (r *FunnelClickHouseRepository) ObjectRelations(ctx context.Context, w string, start, end, watermark time.Time) ([]domain.AnalysisObjectRelation, error) {
	return r.source.ObjectRelations(ctx, w, start, end, watermark)
}
