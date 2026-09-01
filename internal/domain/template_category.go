package domain

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var templateCategoryIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[_-][a-z0-9]+)*$`)

type TemplateCategoryPurpose string

const (
	TemplateCategoryPurposeMarketing     TemplateCategoryPurpose = "marketing"
	TemplateCategoryPurposeTransactional TemplateCategoryPurpose = "transactional"
)

func (p TemplateCategoryPurpose) IsValid() bool {
	return p == TemplateCategoryPurposeMarketing || p == TemplateCategoryPurposeTransactional
}

type TemplateCategoryDefinition struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Purpose    TemplateCategoryPurpose `json:"purpose"`
	SortOrder  int                     `json:"sort_order"`
	IsSystem   bool                    `json:"is_system"`
	IsActive   bool                    `json:"is_active"`
	UsageCount int64                   `json:"usage_count"`
	CreatedAt  time.Time               `json:"created_at"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

func validateTemplateCategoryID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	if len(id) > 20 {
		return errors.New("id must be 20 characters or fewer")
	}
	if !templateCategoryIDPattern.MatchString(id) {
		return errors.New("id must contain lowercase letters and numbers separated by underscores or hyphens")
	}
	return nil
}

func (c TemplateCategoryDefinition) Validate() error {
	if err := validateTemplateCategoryID(c.ID); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if len([]rune(c.Name)) > 64 {
		return errors.New("name must be 64 characters or fewer")
	}
	if !c.Purpose.IsValid() {
		return errors.New("purpose must be marketing or transactional")
	}
	if c.SortOrder < 0 || c.SortOrder > 10000 {
		return errors.New("sort_order must be between 0 and 10000")
	}
	return nil
}

func BuiltInTemplateCategories() []TemplateCategoryDefinition {
	definitions := []TemplateCategoryDefinition{
		{ID: "marketing", Name: "Marketing", Purpose: TemplateCategoryPurposeMarketing, SortOrder: 10},
		{ID: "transactional", Name: "Transactional", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 20},
		{ID: "welcome", Name: "Welcome", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 30},
		{ID: "opt_in", Name: "Opt-in", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 40},
		{ID: "unsubscribe", Name: "Unsubscribe", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 50},
		{ID: "bounce", Name: "Bounce", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 60},
		{ID: "blocklist", Name: "Blocklist", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 70},
		{ID: "blog", Name: "Blog", Purpose: TemplateCategoryPurposeMarketing, SortOrder: 80},
		{ID: "other", Name: "Other", Purpose: TemplateCategoryPurposeTransactional, SortOrder: 90},
	}
	for index := range definitions {
		definitions[index].IsSystem = true
		definitions[index].IsActive = true
	}
	return definitions
}

func EffectiveTemplateCategoryPurpose(categoryID string, resolved TemplateCategoryPurpose) TemplateCategoryPurpose {
	if resolved.IsValid() {
		return resolved
	}
	if categoryID == string(TemplateCategoryMarketing) || categoryID == string(TemplateCategoryBlog) {
		return TemplateCategoryPurposeMarketing
	}
	return TemplateCategoryPurposeTransactional
}

type ListTemplateCategoriesRequest struct {
	WorkspaceID     string `json:"workspace_id"`
	IncludeInactive bool   `json:"include_inactive,omitempty"`
}

func (r ListTemplateCategoriesRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	return nil
}

type CreateTemplateCategoryRequest struct {
	WorkspaceID string                  `json:"workspace_id"`
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Purpose     TemplateCategoryPurpose `json:"purpose"`
	SortOrder   int                     `json:"sort_order"`
}

func (r *CreateTemplateCategoryRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	return (TemplateCategoryDefinition{ID: r.ID, Name: r.Name, Purpose: r.Purpose, SortOrder: r.SortOrder}).Validate()
}

type UpdateTemplateCategoryRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	SortOrder   int    `json:"sort_order"`
	IsActive    bool   `json:"is_active"`
}

func (r *UpdateTemplateCategoryRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	if err := validateTemplateCategoryID(r.ID); err != nil {
		return err
	}
	if r.Name == "" {
		return errors.New("name is required")
	}
	if len([]rune(r.Name)) > 64 {
		return errors.New("name must be 64 characters or fewer")
	}
	if r.SortOrder < 0 || r.SortOrder > 10000 {
		return errors.New("sort_order must be between 0 and 10000")
	}
	return nil
}

type DeleteTemplateCategoryRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *DeleteTemplateCategoryRequest) Validate() error {
	if strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	r.ID = strings.TrimSpace(r.ID)
	return validateTemplateCategoryID(r.ID)
}

var (
	ErrTemplateCategoryNotFound = errors.New("template category not found")
	ErrTemplateCategoryInUse    = errors.New("template category is in use")
	ErrTemplateCategorySystem   = errors.New("system template category cannot be deleted")
)

type TemplateCategoryRepository interface {
	List(ctx context.Context, workspaceID string, includeInactive bool) ([]TemplateCategoryDefinition, error)
	Get(ctx context.Context, workspaceID, id string) (*TemplateCategoryDefinition, error)
	Create(ctx context.Context, workspaceID string, category *TemplateCategoryDefinition) error
	Update(ctx context.Context, workspaceID string, category *TemplateCategoryDefinition) error
	Delete(ctx context.Context, workspaceID, id string) error
}

type TemplateCategoryService interface {
	List(ctx context.Context, request ListTemplateCategoriesRequest) ([]TemplateCategoryDefinition, error)
	Create(ctx context.Context, request CreateTemplateCategoryRequest) (*TemplateCategoryDefinition, error)
	Update(ctx context.Context, request UpdateTemplateCategoryRequest) (*TemplateCategoryDefinition, error)
	Delete(ctx context.Context, request DeleteTemplateCategoryRequest) error
}

func NewTemplateCategoryConflictError(id string, cause error) error {
	return fmt.Errorf("template category %s: %w", id, cause)
}
