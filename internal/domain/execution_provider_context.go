package domain

import (
	"context"
	"fmt"
)

type executionProvidersContextKey struct{}

// ExecutionProviders is stored only within the encrypted internal credential archive.
type ExecutionProviders struct {
	Integrations             Integrations `json:"integrations"`
	MarketingEmailProviderID string       `json:"marketing_email_provider_id"`
}

func WithExecutionProviders(ctx context.Context, providers ExecutionProviders) context.Context {
	return context.WithValue(ctx, executionProvidersContextKey{}, providers)
}

// The current workspace remains the authority for authorization and customer consent.
func ExecutionProviderWorkspace(ctx context.Context, current *Workspace) *Workspace {
	providers, ok := ctx.Value(executionProvidersContextKey{}).(ExecutionProviders)
	if !ok || current == nil {
		return current
	}
	copy := *current
	copy.Integrations = providers.Integrations
	copy.Settings.MarketingEmailProviderID = providers.MarketingEmailProviderID
	return &copy
}

func ExecutionEmailProviderFromContext(ctx context.Context) (*EmailProvider, string, bool, error) {
	providers, ok := ctx.Value(executionProvidersContextKey{}).(ExecutionProviders)
	if !ok {
		return nil, "", false, nil
	}
	workspace := &Workspace{Integrations: providers.Integrations, Settings: WorkspaceSettings{MarketingEmailProviderID: providers.MarketingEmailProviderID}}
	provider, id, err := workspace.GetEmailProviderWithIntegrationID(true)
	if err == nil && provider == nil {
		err = fmt.Errorf("execution email provider is unavailable")
	}
	return provider, id, true, err
}
