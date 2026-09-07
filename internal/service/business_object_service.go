package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type BusinessObjectService struct {
	repo        domain.BusinessObjectRepository
	definitions domain.BusinessDefinitionRepository
	auth        domain.AuthService
}

func NewBusinessObjectService(repo domain.BusinessObjectRepository, definitions domain.BusinessDefinitionRepository, auth domain.AuthService) *BusinessObjectService {
	return &BusinessObjectService{repo, definitions, auth}
}
func (s *BusinessObjectService) authorize(ctx context.Context, w string, write bool) (context.Context, error) {
	return (&TrackedEventService{auth: s.auth}).authorize(ctx, w, write)
}
func (s *BusinessObjectService) Upsert(ctx context.Context, w string, o *domain.BusinessObject) (*domain.BusinessObject, error) {
	ctx, err := s.authorize(ctx, w, true)
	if err != nil {
		return nil, err
	}
	if o.ObjectType == "" || o.Source == "" || o.ExternalKey == "" {
		return nil, domain.NewValidationError("object_type,source,external_key are required")
	}
	if _, err = uuid.Parse(o.CustomerID); err != nil {
		return nil, domain.NewValidationError("customer_id must be a UUID")
	}
	d, err := s.definitions.Get(ctx, w, "object_type", o.ObjectType)
	if err != nil {
		return nil, err
	}
	if d.Status != "active" {
		return nil, domain.NewValidationError("object type must be active")
	}
	if o.Attributes == nil {
		o.Attributes = map[string]any{}
	}
	if err = domain.ValidateBusinessValue(d.Schema, o.Attributes, "attributes"); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}
	o.DefinitionRevision = d.Revision
	return s.repo.Upsert(ctx, w, o)
}
func (s *BusinessObjectService) Get(ctx context.Context, w, typ, source, key string) (*domain.BusinessObject, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, w, typ, source, key)
}
func (s *BusinessObjectService) List(ctx context.Context, w, customer, typ string, limit int) ([]domain.BusinessObject, error) {
	ctx, err := s.authorize(ctx, w, false)
	if err != nil {
		return nil, err
	}
	if customer != "" {
		if _, err = uuid.Parse(customer); err != nil {
			return nil, domain.NewValidationError("customer_id must be a UUID")
		}
	}
	return s.repo.List(ctx, w, customer, typ, limit)
}
func (s *BusinessObjectService) Relate(ctx context.Context, w, key, from, to string) error {
	ctx, err := s.authorize(ctx, w, true)
	if err != nil {
		return err
	}
	if _, err = uuid.Parse(from); err != nil {
		return domain.NewValidationError("from_id must be a UUID")
	}
	if _, err = uuid.Parse(to); err != nil {
		return domain.NewValidationError("to_id must be a UUID")
	}
	d, err := s.definitions.Get(ctx, w, "object_relation", key)
	if err != nil {
		return err
	}
	if d.Status != "active" {
		return domain.NewValidationError("relation must be active")
	}
	return s.repo.Relate(ctx, w, d, from, to)
}
