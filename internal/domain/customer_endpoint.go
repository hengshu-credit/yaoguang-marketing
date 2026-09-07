package domain

import (
	"fmt"
	"github.com/google/uuid"
)

type CustomerEndpointRequest struct {
	ActorID          string                  `json:"-"`
	WorkspaceID      string                  `json:"workspace_id"`
	CustomerID       string                  `json:"customer_id"`
	Endpoint         ContactEndpointMutation `json:"endpoint"`
	MarketingConsent *bool                   `json:"marketing_consent,omitempty"`
}

func (r *CustomerEndpointRequest) Validate() error {
	if r == nil || r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if id, err := uuid.Parse(r.CustomerID); err != nil || id == uuid.Nil {
		return fmt.Errorf("invalid customer_id")
	}
	_, err := r.Endpoint.Validate()
	return err
}
