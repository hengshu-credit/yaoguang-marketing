package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
)

func (r *SubjectStatePostgresRepository) Coverage(ctx context.Context, q domain.SubjectStateCoverageQuery) (domain.SubjectStateCoverage, error) {
	result := domain.SubjectStateCoverage{SubjectType: q.SubjectType, AsOf: q.AsOf, Watermark: q.Watermark, States: []domain.SubjectStateCoverageCount{}}
	db, err := subjectStateDB(ctx, r.base, q.WorkspaceID)
	if err != nil {
		return result, err
	}
	rows, err := db.QueryContext(ctx, `WITH cohort AS (SELECT id::text AS id FROM customers WHERE $1='customer' AND merged_into_id IS NULL AND created_at<=$2 AND created_at<=$3 UNION ALL SELECT id::text FROM business_objects WHERE $1<>'customer' AND object_type=$1 AND created_at<=$2 AND created_at<=$3), eligible_revisions AS (SELECT subject_id,projection_revision FROM subject_state_history WHERE subject_type=$1 GROUP BY subject_id,projection_revision HAVING MAX(computed_at)<=$3), latest AS (SELECT subject_id,MAX(projection_revision) revision FROM eligible_revisions GROUP BY subject_id), values_at AS (SELECT c.id,k.state_key,COALESCE(h.value,'unknown') value FROM cohort c CROSS JOIN unnest($4::text[]) k(state_key) LEFT JOIN latest l ON l.subject_id=c.id LEFT JOIN LATERAL(SELECT value FROM subject_state_history h WHERE h.subject_type=$1 AND h.subject_id=c.id AND h.projection_revision=l.revision AND h.state_key=k.state_key AND h.valid_from<=$2 AND (h.valid_to IS NULL OR h.valid_to>$2) ORDER BY h.valid_from DESC LIMIT 1)h ON true), sets AS (SELECT id,bool_and(value='true') all_true,bool_or(value='true') any_true FROM values_at GROUP BY id) SELECT k.state_key,(SELECT count(*) FROM cohort),(SELECT count(*) FROM sets WHERE all_true),(SELECT count(*) FROM sets WHERE any_true),count(*) FILTER(WHERE v.value='true'),count(*) FILTER(WHERE v.value='false'),count(*) FILTER(WHERE v.value='unknown') FROM unnest($4::text[]) k(state_key) LEFT JOIN values_at v ON v.state_key=k.state_key GROUP BY k.state_key ORDER BY k.state_key`, q.SubjectType, q.AsOf, q.Watermark, pq.Array(q.StateKeys))
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var count domain.SubjectStateCoverageCount
		if err = rows.Scan(&count.StateKey, &result.Total, &result.Intersection, &result.Union, &count.Satisfied, &count.False, &count.Unknown); err != nil {
			return result, err
		}
		result.States = append(result.States, count)
	}
	return result, rows.Err()
}
