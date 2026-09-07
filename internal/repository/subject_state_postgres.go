package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
	"time"
)

type SubjectStatePostgresRepository struct{ base *RealtimePostgresRepository }

func NewSubjectStateRepository(w domain.WorkspaceRepository) *SubjectStatePostgresRepository {
	return &SubjectStatePostgresRepository{base: NewRealtimeRepository(w)}
}
func NewSubjectStateRepositoryWithDB(db *sql.DB) *SubjectStatePostgresRepository {
	return &SubjectStatePostgresRepository{base: NewRealtimeRepositoryWithDB(db)}
}
func (r *SubjectStatePostgresRepository) List(ctx context.Context, w, typ, id string, history bool) ([]domain.SubjectStateInterval, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT h.subject_type,h.subject_id,COALESCE(h.customer_id::text,''),h.state_key,h.value,h.valid_from,h.valid_to,h.rule_revision,h.projection_revision,h.source,h.reason,h.event_ids,h.computed_at FROM subject_state_history h JOIN subject_state_projections p ON p.subject_type=h.subject_type AND p.subject_id=h.subject_id AND p.revision=h.projection_revision WHERE h.subject_type=$1 AND h.subject_id=$2 AND ($3 OR h.valid_to IS NULL) ORDER BY h.state_key,h.valid_from`, typ, id, history)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubjectStates(rows)
}
func (r *SubjectStatePostgresRepository) Overrides(ctx context.Context, w, typ, id string) ([]domain.StateOverride, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT state_key,value,reason,actor,valid_from,valid_to FROM subject_state_overrides WHERE subject_type=$1 AND subject_id=$2 ORDER BY valid_from,id`, typ, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.StateOverride{}
	for rows.Next() {
		var o domain.StateOverride
		if err := rows.Scan(&o.StateKey, &o.Value, &o.Reason, &o.Actor, &o.ValidFrom, &o.ValidTo); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (r *SubjectStatePostgresRepository) SetOverride(ctx context.Context, w, typ, id string, o domain.StateOverride) error {
	if strings.TrimSpace(o.Reason) == "" || o.Actor == "" || o.StateKey == "" || !o.Value.Valid() || o.ValidFrom.IsZero() || typ == "" || id == "" || (o.ValidTo != nil && !o.ValidTo.After(o.ValidFrom)) {
		return domain.NewValidationError("state, subject, value, reason, actor and valid interval required")
	}
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO subject_state_overrides(subject_type,subject_id,state_key,value,reason,actor,valid_from,valid_to) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, typ, id, o.StateKey, o.Value, o.Reason, o.Actor, o.ValidFrom, o.ValidTo)
	return err
}
func (r *SubjectStatePostgresRepository) ReplaceProjection(ctx context.Context, w, typ, id string, intervals []domain.SubjectStateInterval, next *time.Time) error {
	tx, ok := ctx.Value(subjectStateTxKey{}).(*sql.Tx)
	if !ok {
		return r.WithSubjectLock(ctx, w, typ, id, func(locked context.Context) error { return r.ReplaceProjection(locked, w, typ, id, intervals, next) })
	}
	var revision int64
	var previous []domain.SubjectStateInterval
	if domain.StateTransitionEmissionFrom(ctx).Enabled {
		var err error
		previous, err = r.List(ctx, w, typ, id, false)
		if err != nil {
			return err
		}
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,revision,next_check) VALUES($1,$2,1,$3) ON CONFLICT(subject_type,subject_id) DO UPDATE SET revision=subject_state_projections.revision+1,next_check=$3,updated_at=NOW() RETURNING revision`, typ, id, next).Scan(&revision)
	if err != nil {
		return err
	}
	for _, h := range intervals {
		ids, err := json.Marshal(h.EventIDs)
		if err != nil {
			return err
		}
		if string(ids) == "null" {
			ids = []byte("[]")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO subject_state_history(subject_type,subject_id,customer_id,state_key,value,valid_from,valid_to,rule_revision,projection_revision,source,reason,event_ids,computed_at) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, typ, id, h.CustomerID, h.StateKey, h.Value, h.ValidFrom, h.ValidTo, h.RuleRevision, revision, h.Source, h.Reason, ids, h.ComputedAt)
		if err != nil {
			return err
		}
	}
	// A child expiry or recomputation invalidates the owning customer's aggregate
	// even when no new event arrived. The upsert is part of this publication.
	if typ != "customer" && len(intervals) > 0 && intervals[0].CustomerID != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,next_check) VALUES('customer',$1,NOW()) ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW()`, intervals[0].CustomerID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO subject_state_projections(subject_type,subject_id,next_check) SELECT o.object_type,rel.from_id::text,NOW() FROM business_object_relations rel JOIN business_objects o ON o.id=rel.from_id WHERE rel.to_id=$1 AND (rel.valid_to IS NULL OR rel.valid_to>NOW()) ON CONFLICT(subject_type,subject_id) DO UPDATE SET next_check=NOW()`, id); err != nil {
			return err
		}
	}
	if err = appendSubjectStateTransitions(ctx, tx, w, previous, intervals); err != nil {
		return err
	}
	return nil
}
func (r *SubjectStatePostgresRepository) Due(ctx context.Context, w string, at time.Time, limit int) ([]domain.EventSubject, error) {
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT subject_type,subject_id FROM subject_state_projections WHERE next_check <= $1 ORDER BY next_check LIMIT $2`, at, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.EventSubject{}
	for rows.Next() {
		var s domain.EventSubject
		if err := rows.Scan(&s.Type, &s.ID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *SubjectStatePostgresRepository) Context(ctx context.Context, w, typ, id string) (domain.StateEvaluationContext, error) {
	out := domain.StateEvaluationContext{At: time.Now().UTC(), Complete: true, Attributes: map[string]interface{}{}}
	db, err := subjectStateDB(ctx, r.base, w)
	if err != nil {
		return out, err
	}
	var raw []byte
	if typ == "customer" {
		err = db.QueryRowContext(ctx, `SELECT COALESCE(p.attributes,'{}'::jsonb) FROM customers c LEFT JOIN customer_profiles p ON p.customer_id=c.id WHERE c.id=$1 AND c.merged_into_id IS NULL`, id).Scan(&raw)
	} else {
		err = db.QueryRowContext(ctx, `SELECT attributes FROM business_objects WHERE object_type=$1 AND id=$2`, typ, id).Scan(&raw)
	}
	if err == sql.ErrNoRows {
		return out, domain.NewValidationError("subject does not exist")
	}
	if err != nil {
		return out, fmt.Errorf("load state subject: %w", err)
	}
	if err = json.Unmarshal(raw, &out.Attributes); err != nil {
		return out, err
	}
	out.AttributeHistory, err = r.AttributeHistory(ctx, w, typ, id, out.At)
	if err != nil {
		return out, err
	}
	if typ == "customer" {
		rows, err := db.QueryContext(ctx, `SELECT object_type,id::text,attributes,created_at FROM business_objects WHERE customer_id=$1`, id)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		children := map[string]*domain.StateObjectContext{}
		for rows.Next() {
			var st, sid string
			var attributes []byte
			var created time.Time
			if err = rows.Scan(&st, &sid, &attributes, &created); err != nil {
				return out, err
			}
			k := st + ":" + sid
			if children[k] == nil {
				children[k] = &domain.StateObjectContext{SubjectType: st, SubjectID: sid, States: map[string]domain.StateValue{}, ValidFrom: &created}
			}
			if err = json.Unmarshal(attributes, &children[k].Attributes); err != nil {
				return out, err
			}
		}
		if err = rows.Err(); err != nil {
			return out, err
		}
		rows.Close()
		historyRows, err := db.QueryContext(ctx, `SELECT h.subject_type,h.subject_id,COALESCE(h.customer_id::text,''),h.state_key,h.value,h.valid_from,h.valid_to,h.rule_revision,h.projection_revision,h.source,h.reason,h.event_ids,h.computed_at FROM subject_state_history h JOIN subject_state_projections p ON p.subject_type=h.subject_type AND p.subject_id=h.subject_id AND p.revision=h.projection_revision WHERE h.customer_id=$1 AND h.subject_type<>'customer'`, id)
		if err != nil {
			return out, err
		}
		out.ChildHistory, err = scanSubjectStates(historyRows)
		historyRows.Close()
		if err != nil {
			return out, err
		}
		for _, h := range out.ChildHistory {
			if c := children[h.SubjectType+":"+h.SubjectID]; c != nil && !out.At.Before(h.ValidFrom) && (h.ValidTo == nil || out.At.Before(*h.ValidTo)) {
				c.States[h.StateKey] = h.Value
			}
		}
		ownedRelations, relationErr := (&FunnelPostgresRepository{base: r.base}).ObjectRelations(ctx, w, time.Unix(0, 0).UTC(), out.At, out.At)
		if relationErr != nil {
			return out, relationErr
		}
		for _, c := range children {
			c.AttributeHistory, err = r.AttributeHistory(ctx, w, c.SubjectType, c.SubjectID, out.At)
			if err != nil {
				return out, err
			}
			c.Events, err = (&FunnelPostgresRepository{base: r.base}).EventsForSubject(ctx, w, c.SubjectType, c.SubjectID, time.Unix(0, 0).UTC(), out.At, out.At)
			if err != nil {
				return out, err
			}
			out.Children = append(out.Children, *c)
			for _, relation := range ownedRelations {
				if relation.FromType == "customer" && relation.FromID == id && relation.ToType == c.SubjectType && relation.ToID == c.SubjectID {
					linked := *c
					linked.RelationKey = relation.Key
					linked.ValidFrom = &relation.ValidFrom
					linked.ValidTo = relation.ValidTo
					out.Children = append(out.Children, linked)
				}
			}
		}
	} else {
		out.Children, out.ChildHistory, err = r.relatedObjectContext(ctx, w, id, out.At)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
