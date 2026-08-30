package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CustomerReconciliationJobType string

const (
	CustomerReconciliationScan   CustomerReconciliationJobType = "scan"
	CustomerReconciliationRepair CustomerReconciliationJobType = "repair"
)

type CustomerReconciliationStatus string

const (
	CustomerReconciliationRunning   CustomerReconciliationStatus = "running"
	CustomerReconciliationCompleted CustomerReconciliationStatus = "completed"
	CustomerReconciliationFailed    CustomerReconciliationStatus = "failed"
)

type CustomerReconciliationRequest struct {
	WorkspaceID string                        `json:"workspace_id"`
	JobType     CustomerReconciliationJobType `json:"-"`
}

func (request *CustomerReconciliationRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if request.JobType != CustomerReconciliationScan && request.JobType != CustomerReconciliationRepair {
		return fmt.Errorf("job_type must be scan or repair")
	}
	return nil
}

type CustomerReconciliationGetRequest struct {
	WorkspaceID string `json:"workspace_id"`
	RunID       string `json:"run_id,omitempty"`
}

func (request *CustomerReconciliationGetRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.RunID = strings.TrimSpace(request.RunID)
	if request.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if request.RunID != "" {
		parsed, err := uuid.Parse(request.RunID)
		if err != nil || parsed == uuid.Nil {
			return fmt.Errorf("run_id must be a non-nil UUID")
		}
		request.RunID = parsed.String()
	}
	return nil
}

type CustomerReconciliationFinding struct {
	EntityName        string `json:"entity_name"`
	MissingCount      int64  `json:"missing_count"`
	ConflictCount     int64  `json:"conflict_count"`
	RepairableCount   int64  `json:"repairable_count"`
	RepairedCount     int64  `json:"repaired_count"`
	UnrepairableCount int64  `json:"unrepairable_count"`
}

type CustomerReconciliationRun struct {
	ID            string                          `json:"run_id"`
	JobType       CustomerReconciliationJobType   `json:"job_type"`
	Status        CustomerReconciliationStatus    `json:"status"`
	BatchSize     int                             `json:"batch_size"`
	Checkpoint    map[string]string               `json:"checkpoint"`
	Findings      []CustomerReconciliationFinding `json:"findings"`
	MissingCount  int64                           `json:"missing_count"`
	ConflictCount int64                           `json:"conflict_count"`
	RepairedCount int64                           `json:"repaired_count"`
	LastError     string                          `json:"last_error,omitempty"`
	StartedAt     time.Time                       `json:"started_at"`
	UpdatedAt     time.Time                       `json:"updated_at"`
	CompletedAt   *time.Time                      `json:"completed_at,omitempty"`
}

type CustomerReconciliationRepository interface {
	Run(ctx context.Context, workspaceID string, jobType CustomerReconciliationJobType, batchSize int) (*CustomerReconciliationRun, error)
	Get(ctx context.Context, workspaceID, runID string) (*CustomerReconciliationRun, error)
}

type CustomerReconciliationService interface {
	Scan(ctx context.Context, request *CustomerReconciliationRequest) (*CustomerReconciliationRun, error)
	Repair(ctx context.Context, request *CustomerReconciliationRequest) (*CustomerReconciliationRun, error)
	Get(ctx context.Context, request *CustomerReconciliationGetRequest) (*CustomerReconciliationRun, error)
}

type ErrCustomerReconciliationNotFound struct {
	RunID string
}

func (err *ErrCustomerReconciliationNotFound) Error() string {
	if err.RunID == "" {
		return "customer reconciliation run not found"
	}
	return fmt.Sprintf("customer reconciliation run %s not found", err.RunID)
}
