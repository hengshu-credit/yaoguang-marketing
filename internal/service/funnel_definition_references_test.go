package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"testing"
)

func TestFunnelDefinitionChecksRelatedStepReferences(t *testing.T) {
	defs := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{}}
	for _, typ := range []string{"application", "order", "other"} {
		defs.defs["object_type:"+typ] = &domain.BusinessDefinition{Kind: "object_type", Key: typ, Status: "active"}
	}
	defs.defs["object_relation:orders"] = &domain.BusinessDefinition{Kind: "object_relation", Key: "orders", Status: "active", Settings: map[string]any{"from_type": "application", "to_type": "order"}}
	for code, typ := range map[string]string{"apply": "application", "paid": "order"} {
		defs.defs["event:"+code] = &domain.BusinessDefinition{Kind: "event", Key: code, Status: "active", Settings: map[string]any{"subject_type": typ}}
	}
	service := NewBusinessConfigService(defs, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	d := domain.FunnelDefinition{SubjectType: "application", WindowSeconds: 60, Steps: []domain.FunnelStep{{ID: "a", Events: []string{"apply"}}, {ID: "b", SubjectType: "order", RelationKey: "orders", Events: []string{"paid"}}}}
	makeDef := func() *domain.BusinessDefinition {
		raw, _ := json.Marshal(d)
		settings := map[string]any{}
		_ = json.Unmarshal(raw, &settings)
		return &domain.BusinessDefinition{Kind: "funnel", Key: "linked", Name: "Linked", Status: "active", Settings: settings}
	}
	if _, err := service.Save(context.Background(), "w", makeDef()); err != nil {
		t.Fatal(err)
	}
	defs.defs["object_relation:orders"].Settings["from_type"] = "other"
	if _, err := service.Save(context.Background(), "w", makeDef()); err == nil {
		t.Fatal("relation from an unbound parent accepted")
	}
	defs.defs["object_relation:orders"].Settings["from_type"] = "application"
	defs.defs["event:paid"].Settings["subject_type"] = "other"
	if _, err := service.Save(context.Background(), "w", makeDef()); err == nil {
		t.Fatal("wrong event object accepted")
	}
}
