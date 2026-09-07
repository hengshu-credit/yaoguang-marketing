package domain

import (
	"context"
	"time"
)

type FunnelQueryJob struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"`
	Query     FunnelQuery   `json:"query"`
	Result    *FunnelResult `json:"result,omitempty"`
	Error     string        `json:"error,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}
type FunnelQueryJobRepository interface {
	CreateQuery(context.Context, string, *FunnelQueryJob) error
	GetQuery(context.Context, string, string) (*FunnelQueryJob, error)
	FinishQuery(context.Context, string, string, string, *FunnelResult, string) error
}
