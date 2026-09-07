package domain

import (
	"context"
	"time"
)

type BusinessObject struct {
	ID                 string         `json:"id"`
	ObjectType         string         `json:"object_type"`
	Source             string         `json:"source"`
	ExternalKey        string         `json:"external_key"`
	CustomerID         string         `json:"customer_id"`
	Attributes         map[string]any `json:"attributes"`
	DefinitionRevision int            `json:"definition_revision"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}
type BusinessObjectRepository interface {
	Upsert(context.Context, string, *BusinessObject) (*BusinessObject, error)
	Get(context.Context, string, string, string, string) (*BusinessObject, error)
	List(context.Context, string, string, string, int) ([]BusinessObject, error)
	Relate(context.Context, string, *BusinessDefinition, string, string) error
}
