package repository

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

type FunnelEventReader interface {
	Events(context.Context, string, string, time.Time, time.Time, time.Time) ([]domain.AnalysisEvent, error)
}

// ClickHouse accelerates fact reads; state intervals and verified attribution
// stay on the authoritative workspace database and use the same evaluator.
type FunnelClickHouseRepository struct {
	reader FunnelEventReader
	source *FunnelPostgresRepository
}

func NewFunnelClickHouseRepository(reader FunnelEventReader, source *FunnelPostgresRepository) *FunnelClickHouseRepository {
	return &FunnelClickHouseRepository{reader: reader, source: source}
}
func (r *FunnelClickHouseRepository) Events(c context.Context, w, s string, a, b, m time.Time) ([]domain.AnalysisEvent, error) {
	return r.reader.Events(c, w, s, a, b, m)
}
func (r *FunnelClickHouseRepository) StateHistory(c context.Context, w string, a, b time.Time) ([]domain.SubjectStateInterval, error) {
	return r.source.StateHistory(c, w, a, b)
}
func (r *FunnelClickHouseRepository) AttributionLinks(c context.Context, w string, a, b, m time.Time) ([]domain.AnalysisTouchpoint, error) {
	return r.source.AttributionLinks(c, w, a, b, m)
}
