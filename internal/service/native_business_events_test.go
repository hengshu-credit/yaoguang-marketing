package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativeBusinessEventsCannotBeClaimedAsCustom(t *testing.T) {
	for _, channel := range []string{"sms", "push", "whatsapp", "webhook"} {
		for _, suffix := range []string{"sent", "delivered", "opened", "clicked", "failed", "bounced", "complained", "unsubscribed", "updated"} {
			t.Run(channel+"."+suffix, func(t *testing.T) {
				repo := &businessTestDefinitions{defs: map[string]*domain.BusinessDefinition{}}
				service := NewBusinessConfigService(repo, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
				def := &domain.BusinessDefinition{Kind: "event", Key: channel + "." + suffix, Name: "Override", Status: "active", Schema: map[string]any{"type": "object"}, Settings: map[string]any{"expiry_mode": "never"}}
				_, err := service.Save(context.Background(), "w", def)
				require.ErrorContains(t, err, "Built-in")
				def.Key = channel + ".custom_business_action"
				_, err = service.Save(context.Background(), "w", def)
				require.NoError(t, err)
			})
		}
	}
}

func TestNativeBusinessEventTrackingCustomerScope(t *testing.T) {
	defs := configuredDefinitionStub{values: map[string]*domain.BusinessDefinition{"object_type:contract": {Status: "active"}}}
	for _, code := range []string{"sms.sent", "push.clicked", "whatsapp.delivered", "webhook.failed"} {
		defs.values["event:"+code] = &domain.BusinessDefinition{Status: "active", Settings: map[string]any{"subject_type": "customer", "builtin": true}}
		plan := domain.MarketingTrackingPlan{SubjectType: "customer", Events: []string{code}, WindowSeconds: 3600}
		require.NoError(t, ValidateTaskTracking(context.Background(), "w", defs, plan))
		plan.SubjectType = "contract"
		require.ErrorContains(t, ValidateTaskTracking(context.Background(), "w", defs, plan), "subject type")
	}
}
