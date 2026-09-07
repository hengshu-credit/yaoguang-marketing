package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *SubjectStatePostgresRepository) relatedObjectContext(ctx context.Context, w, id string, at time.Time) ([]domain.StateObjectContext, []domain.SubjectStateInterval, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.QueryContext(ctx, `WITH latest AS (SELECT DISTINCT ON(relation_id) * FROM subject_relation_history WHERE recorded_at<=$2 ORDER BY relation_id,recorded_at DESC,id DESC) SELECT o.object_type,o.id::text,o.attributes,rel.relation_key,rel.valid_from,rel.valid_to FROM latest rel JOIN business_objects o ON o.id::text=rel.to_id WHERE rel.from_id=$1 AND rel.from_type<>'customer' ORDER BY rel.valid_from,rel.id LIMIT 10001`, id, at)
	if err != nil {
		return nil, nil, err
	}
	children := []domain.StateObjectContext{}
	for rows.Next() {
		var child domain.StateObjectContext
		var raw []byte
		if err = rows.Scan(&child.SubjectType, &child.SubjectID, &raw, &child.RelationKey, &child.ValidFrom, &child.ValidTo); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if err = json.Unmarshal(raw, &child.Attributes); err != nil {
			rows.Close()
			return nil, nil, err
		}
		children = append(children, child)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(children) > 10000 {
		return nil, nil, fmt.Errorf("related object limit exceeded")
	}
	allHistory := []domain.SubjectStateInterval{}
	for i := range children {
		child := &children[i]
		child.AttributeHistory, err = r.AttributeHistory(ctx, w, child.SubjectType, child.SubjectID, at)
		if err != nil {
			return nil, nil, err
		}
		child.Events, err = (&FunnelPostgresRepository{base: r.base}).EventsForSubject(ctx, w, child.SubjectType, child.SubjectID, time.Unix(0, 0).UTC(), at, at)
		if err != nil {
			return nil, nil, err
		}
		history, err := r.List(ctx, w, child.SubjectType, child.SubjectID, true)
		if err != nil {
			return nil, nil, err
		}
		child.States = map[string]domain.StateValue{}
		for _, h := range history {
			if !at.Before(h.ValidFrom) && (h.ValidTo == nil || at.Before(*h.ValidTo)) {
				child.States[h.StateKey] = h.Value
			}
		}
		allHistory = append(allHistory, history...)
	}
	return children, allHistory, nil
}
