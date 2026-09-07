package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

func ValidateTaskTracking(ctx context.Context, workspace string, definitions domain.BusinessDefinitionRepository, raw any) error {
	if raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return domain.NewValidationError("tracking plan must be an object")
	}
	var plan domain.MarketingTrackingPlan
	if err = json.Unmarshal(encoded, &plan); err != nil {
		return domain.NewValidationError("tracking plan must be an object")
	}
	if err = plan.Validate(); err != nil {
		return domain.NewValidationError(err.Error())
	}
	if definitions == nil {
		return domain.NewValidationError("tracking definitions are unavailable")
	}
	return NewConfigurableAutomationValidator(nil, definitions).ValidateAutomationNodes(ctx, workspace, &domain.Automation{Trigger: &domain.TimelineTriggerConfig{TrackingPlan: &plan}})
}

func (s *BroadcastService) SetBusinessDefinitions(definitions domain.BusinessDefinitionRepository) {
	s.businessDefinitions = definitions
}
