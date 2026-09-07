package domain

import (
	"context"
	"encoding/json"
	"fmt"
)

type executionSnapshotContextKey struct{}
type boundExecutionSnapshot struct {
	ID            string
	Kind          string
	SourceID      string
	Templates     []*Template
	OriginEventID string
}

func WithExecutionSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) (context.Context, error) {
	var templates []*Template
	if len(snapshot.Materials) > 0 && snapshot.Materials[0] == '[' {
		if err := json.Unmarshal(snapshot.Materials, &templates); err != nil {
			return ctx, err
		}
	} else {
		var materials struct {
			Templates []*Template `json:"templates"`
		}
		if err := json.Unmarshal(snapshot.Materials, &materials); err != nil {
			return ctx, err
		}
		templates = materials.Templates
	}
	var dependencies struct {
		Enrollment struct {
			OriginEventID string `json:"origin_event_id"`
		} `json:"enrollment"`
	}
	_ = json.Unmarshal(snapshot.Dependencies, &dependencies)
	return context.WithValue(ctx, executionSnapshotContextKey{}, boundExecutionSnapshot{ID: snapshot.ExecutionID, Kind: snapshot.Kind, SourceID: snapshot.SourceID, Templates: templates, OriginEventID: dependencies.Enrollment.OriginEventID}), nil
}
func ExecutionIDFromContext(ctx context.Context) string {
	bound, _ := ctx.Value(executionSnapshotContextKey{}).(boundExecutionSnapshot)
	return bound.ID
}
func ExecutionTemplateFromContext(ctx context.Context, id string, version int64) (*Template, bool, error) {
	bound, ok := ctx.Value(executionSnapshotContextKey{}).(boundExecutionSnapshot)
	if !ok {
		return nil, false, nil
	}
	var selected *Template
	for _, template := range bound.Templates {
		if template.ID == id && (version == 0 || template.Version == version) && (selected == nil || template.Version > selected.Version) {
			selected = template
		}
	}
	if template := selected; template != nil {
		body, err := json.Marshal(template)
		if err != nil {
			return nil, true, err
		}
		var copy Template
		err = json.Unmarshal(body, &copy)
		return &copy, true, err
	}
	return nil, true, fmt.Errorf("template %s version %d was not captured for execution %s", id, version, bound.ID)
}

func WithExecutionCausation(ctx context.Context, origin string) context.Context {
	bound, ok := ctx.Value(executionSnapshotContextKey{}).(boundExecutionSnapshot)
	if !ok {
		return ctx
	}
	bound.OriginEventID = origin
	return context.WithValue(ctx, executionSnapshotContextKey{}, bound)
}
func ExecutionMetadataFromContext(ctx context.Context) map[string]interface{} {
	bound, ok := ctx.Value(executionSnapshotContextKey{}).(boundExecutionSnapshot)
	if !ok {
		return nil
	}
	metadata := map[string]interface{}{"execution_id": bound.ID}
	if bound.Kind == "automation" || bound.Kind == "journey" {
		metadata["automation_id"] = bound.SourceID
	}
	if bound.OriginEventID != "" {
		metadata["causation_id"] = bound.OriginEventID
		metadata["origin_event_id"] = bound.OriginEventID
	}
	return metadata
}
