package domain

import "time"

type SubjectStateCoverageQuery struct {
	WorkspaceID string    `json:"workspace_id"`
	SubjectType string    `json:"subject_type"`
	AsOf        time.Time `json:"as_of"`
	Watermark   time.Time `json:"watermark"`
	StateKeys   []string  `json:"selected_state_keys"`
}
type SubjectStateCoverageCount struct {
	StateKey  string `json:"state_key"`
	Satisfied int64  `json:"satisfied"`
	False     int64  `json:"false"`
	Unknown   int64  `json:"unknown"`
}
type SubjectStateCoverage struct {
	SubjectType  string                      `json:"subject_type"`
	AsOf         time.Time                   `json:"as_of"`
	Watermark    time.Time                   `json:"watermark"`
	Total        int64                       `json:"total"`
	Intersection int64                       `json:"intersection"`
	Union        int64                       `json:"union"`
	States       []SubjectStateCoverageCount `json:"states"`
}
