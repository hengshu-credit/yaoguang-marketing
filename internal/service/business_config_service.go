package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"strings"
	"time"
)

type BusinessConfigService struct {
	repo      domain.BusinessDefinitionRepository
	auth      domain.AuthService
	customers domain.CustomerRepository
}

func (s *BusinessConfigService) SetCustomerRepository(repo domain.CustomerRepository) {
	s.customers = repo
}

func NewBusinessConfigService(repo domain.BusinessDefinitionRepository, auth domain.AuthService) *BusinessConfigService {
	return &BusinessConfigService{repo: repo, auth: auth}
}
func (s *BusinessConfigService) authorize(ctx context.Context, w string, write bool) (context.Context, error) {
	if w == "" {
		return ctx, domain.NewValidationError("workspace_id is required")
	}
	ctx, _, member, err := s.auth.AuthenticateUserForWorkspace(ctx, w)
	if err != nil {
		return ctx, err
	}
	permission := domain.PermissionTypeRead
	resource := domain.PermissionResourceContacts
	if write {
		resource = domain.PermissionResourceWorkspace
		permission = domain.PermissionTypeWrite
	}
	if member == nil || !member.HasPermission(resource, permission) {
		return ctx, domain.NewPermissionError(resource, permission, "Business configuration access denied")
	}
	return ctx, nil
}
func (s *BusinessConfigService) List(ctx context.Context, w string, f domain.BusinessDefinitionFilter) ([]domain.BusinessDefinition, int, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, w, f)
}
func (s *BusinessConfigService) Get(ctx context.Context, w, kind, key string) (*domain.BusinessDefinition, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, w, kind, key)
}
func (s *BusinessConfigService) ExtendEvents(ctx context.Context, workspace string, keys []string, schema map[string]any) ([]domain.BusinessDefinition, error) {
	ctx, err := s.authorize(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 || len(keys) > 1000 {
		return nil, domain.NewValidationError("select between 1 and 1000 events")
	}
	if _, err := domain.ExtendEventSchema(map[string]any{"type": "object"}, schema); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	unique := []string{}
	for _, key := range keys {
		if key == "" {
			return nil, domain.NewValidationError("event key is required")
		}
		if !seen[key] {
			unique = append(unique, key)
			seen[key] = true
		}
	}
	repository, ok := s.repo.(interface {
		ExtendEvents(context.Context, string, []string, map[string]any) ([]domain.BusinessDefinition, error)
	})
	if !ok {
		return nil, fmt.Errorf("event field extension repository is unavailable")
	}
	return repository.ExtendEvents(ctx, workspace, unique, schema)
}

func (s *BusinessConfigService) Save(ctx context.Context, w string, d *domain.BusinessDefinition) (*domain.BusinessDefinition, error) {
	ctx, err := s.authorize(ctx, w, true)
	if err != nil {
		return nil, err
	}
	if err = d.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	if d.Kind == "event" && domain.IsNativeBusinessEvent(d.Key) {
		return nil, domain.NewValidationError("Built-in event semantics cannot be changed")
	}
	old, err := s.repo.Get(ctx, w, d.Kind, d.Key)
	if err != nil && !errors.Is(err, domain.ErrBusinessDefinitionNotFound) {
		return nil, err
	}
	if old != nil && old.Settings["builtin"] == true {
		return nil, domain.NewValidationError("Built-in event semantics cannot be changed")
	}
	if d.Settings["builtin"] == true {
		return nil, domain.NewValidationError("Built-in marker is reserved")
	}
	if d.Status == "active" {
		if err = s.validateReferences(ctx, w, d); err != nil {
			return nil, domain.NewValidationError(err.Error())
		}
	}
	return s.repo.Save(ctx, w, d)
}
func (s *BusinessConfigService) validateReferences(ctx context.Context, w string, d *domain.BusinessDefinition) error {
	check := func(key string) error {
		if key == "" || key == "customer" {
			return nil
		}
		ref, err := s.repo.Get(ctx, w, "object_type", key)
		if err != nil || ref.Status != "active" {
			return fmt.Errorf("object type %q must be active", key)
		}
		return nil
	}
	if d.Kind == "event" {
		subject, _ := d.Settings["subject_type"].(string)
		return check(subject)
	}
	if d.Kind == "funnel" {
		var definition domain.FunnelDefinition
		raw, _ := json.Marshal(d.Settings)
		if err := json.Unmarshal(raw, &definition); err != nil {
			return err
		}
		if err := definition.Validate(); err != nil {
			return err
		}
		if err := check(definition.SubjectType); err != nil {
			return err
		}
		return s.validateFunnelReferences(ctx, w, definition)
	}
	if d.Kind == "state" {
		entries, _, err := s.repo.List(ctx, w, domain.BusinessDefinitionFilter{Kind: "state", Status: "active", Limit: 1000})
		if err != nil {
			return err
		}
		definitions := []domain.SubjectStateDefinition{}
		for i, entry := range append(entries, *d) {
			if entry.Key == d.Key && i < len(entries) {
				continue
			}
			var state domain.SubjectStateDefinition
			raw, _ := json.Marshal(entry.Settings)
			if err = json.Unmarshal(raw, &state); err != nil {
				return err
			}
			state.Key = entry.Key
			state.Revision = entry.Revision
			if entry.Key == d.Key {
				if err = check(state.SubjectType); err != nil {
					return err
				}
				if err = s.validateAnalysisRuleReferences(ctx, w, state.SubjectType, state.Rule); err != nil {
					return err
				}
			}
			definitions = append(definitions, state)
		}
		return domain.ValidateSubjectStateDefinitions(definitions)
	}
	if d.Kind == "object_relation" {
		from, _ := d.Settings["from_type"].(string)
		to, _ := d.Settings["to_type"].(string)
		if err := check(from); err != nil {
			return err
		}
		if err := check(to); err != nil {
			return err
		}
		defs, _, err := s.repo.List(ctx, w, domain.BusinessDefinitionFilter{Kind: "object_relation", Status: "active", Limit: 1000})
		if err != nil {
			return err
		}
		edges := map[string][]string{}
		for _, r := range defs {
			if r.Key == d.Key {
				continue
			}
			a, _ := r.Settings["from_type"].(string)
			b, _ := r.Settings["to_type"].(string)
			edges[a] = append(edges[a], b)
		}
		edges[from] = append(edges[from], to)
		visiting := map[string]bool{}
		done := map[string]bool{}
		var visit func(string) bool
		visit = func(k string) bool {
			if visiting[k] {
				return true
			}
			if done[k] {
				return false
			}
			visiting[k] = true
			for _, next := range edges[k] {
				if visit(next) {
					return true
				}
			}
			visiting[k] = false
			done[k] = true
			return false
		}
		for key := range edges {
			if visit(key) {
				return fmt.Errorf("object relation dependency cycle")
			}
		}
	}
	return nil
}
func (s *BusinessConfigService) Archive(ctx context.Context, w, kind, key string, revision int) (*domain.BusinessDefinition, error) {
	d, err := s.Get(ctx, w, kind, key)
	if err != nil {
		return nil, err
	}
	d.Status = "archived"
	d.Revision = revision
	return s.Save(ctx, w, d)
}

type BusinessPreviewError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
type BusinessPreviewResult struct {
	Valid              bool                   `json:"valid"`
	Errors             []BusinessPreviewError `json:"errors"`
	NormalizedEvent    any                    `json:"normalized_event,omitempty"`
	ResolvedCustomerID string                 `json:"resolved_customer_id,omitempty"`
	AttributeChanges   []map[string]any       `json:"attribute_changes,omitempty"`
}

func (s *BusinessConfigService) Preview(ctx context.Context, w string, d *domain.BusinessDefinition, sample map[string]any) (*BusinessPreviewResult, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	result := &BusinessPreviewResult{Valid: true, Errors: []BusinessPreviewError{}}
	if err = d.Validate(); err == nil {
		switch d.Kind {
		case "customer_attribute":
			err = domain.ValidateBusinessValue(d.Schema, sample["value"], "sample.value")
		case "event", "event_mapping":
			definition := d
			var mapping *domain.BusinessDefinition
			if d.Kind == "event_mapping" {
				mapping = d
				var mapped map[string]any
				mapped, err = domain.MapTrackedEvent(sample, mapping)
				if err == nil {
					code, _ := mapped["event_code"].(string)
					definition, err = s.repo.Get(ctx, w, "event", code)
				}
			}
			if err == nil {
				var event *domain.TrackedEvent
				event, err = domain.NormalizeTrackedEvent(sample, definition, mapping, time.Now().UTC())
				if err == nil {
					result.NormalizedEvent = event
					if mapping != nil {
						var rules []domain.AttributeMapping
						bytes, _ := json.Marshal(mapping.Settings["attribute_mappings"])
						_ = json.Unmarshal(bytes, &rules)
						input := map[string]any{"data": event.Data, "customer_attributes": event.CustomerAttributes}
						for _, rule := range rules {
							if value, ok := domain.BusinessJSONPath(input, rule.Path); ok {
								result.AttributeChanges = append(result.AttributeChanges, map[string]any{"attribute": rule.Attribute, "action": rule.Action, "value": value, "null_policy": rule.NullPolicy, "update_policy": rule.UpdatePolicy})
							}
						}
					}
					if s.customers != nil {
						for key, value := range event.CustomerAttributes {
							text, ok := value.(string)
							if !ok {
								continue
							}
							locator := domain.CustomerLocator{}
							switch key {
							case "customer_id":
								locator.CustomerID = text
							case "external_user_id":
								locator.ExternalUserID = text
							case "email", "phone":
								locator.Identity = &domain.CustomerIdentityLocator{Type: domain.CustomerIdentityType(key), Value: text}
							default:
								attribute, lookupErr := s.repo.Get(ctx, w, "customer_attribute", key)
								if errors.Is(lookupErr, domain.ErrBusinessDefinitionNotFound) {
									continue
								}
								if lookupErr != nil {
									err = lookupErr
									break
								}
								if attribute.Status != "active" || attribute.Settings["identity"] != true {
									continue
								}
								switch attribute.Settings["normalization"] {
								case "lowercase":
									text = strings.ToLower(strings.TrimSpace(text))
								case "trim":
									text = strings.TrimSpace(text)
								case "phone":
									normalized, normalizeErr := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: domain.CustomerIdentityPhone, Value: text})
									if normalizeErr != nil {
										err = normalizeErr
										break
									}
									text = normalized.Value
								}
								if err != nil {
									break
								}
								if err = domain.ValidateBusinessValue(attribute.Schema, text, "customer_attributes."+key); err != nil {
									break
								}
								locator.Identity = &domain.CustomerIdentityLocator{Type: domain.CustomerIdentityCustom, Value: key + ":" + text}
							}
							if err != nil {
								break
							}
							customer, lookupErr := s.customers.Get(ctx, w, locator)
							if lookupErr == nil {
								if result.ResolvedCustomerID != "" && result.ResolvedCustomerID != customer.ID {
									err = fmt.Errorf("customer attributes resolve to conflicting customers")
									break
								}
								result.ResolvedCustomerID = customer.ID
							} else {
								var missing *domain.ErrCustomerNotFound
								if !errors.As(lookupErr, &missing) {
									err = lookupErr
									break
								}
							}
						}
					}
				}
			}
		default:
			err = domain.ValidateBusinessValue(d.Schema, sample, "sample")
		}
	}
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, BusinessPreviewError{Field: "sample", Message: err.Error()})
	}
	return result, nil
}
