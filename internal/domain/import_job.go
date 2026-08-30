package domain

import (
	"errors"
	"fmt"
	"time"
)

type ImportJobStatus string
type ImportRowStatus string

const (
	ImportJobUploading  ImportJobStatus = "uploading"
	ImportJobStaged     ImportJobStatus = "staged"
	ImportJobProcessing ImportJobStatus = "processing"
	ImportJobCompleted  ImportJobStatus = "completed"
	ImportJobRejected   ImportJobStatus = "rejected"
	ImportJobCancelled  ImportJobStatus = "cancelled"
	ImportRowPending    ImportRowStatus = "pending"
	ImportRowProcessing ImportRowStatus = "processing"
	ImportRowSucceeded  ImportRowStatus = "succeeded"
	ImportRowFailed     ImportRowStatus = "failed"
)

type ImportCounters struct {
	Total      int64 `json:"total"`
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Succeeded  int64 `json:"succeeded"`
	Failed     int64 `json:"failed"`
}

func (c ImportCounters) Validate() error {
	if c.Total < 0 || c.Pending < 0 || c.Processing < 0 || c.Succeeded < 0 || c.Failed < 0 {
		return errors.New("import counters cannot be negative")
	}
	if c.Total != c.Pending+c.Processing+c.Succeeded+c.Failed {
		return fmt.Errorf("import row conservation violated: total=%d states=%d", c.Total, c.Pending+c.Processing+c.Succeeded+c.Failed)
	}
	return nil
}

func (c ImportCounters) Transition(from, to ImportRowStatus) (ImportCounters, error) {
	if from == to {
		return c, c.Validate()
	}
	adjust := func(status ImportRowStatus, delta int64) error {
		switch status {
		case ImportRowPending:
			c.Pending += delta
		case ImportRowProcessing:
			c.Processing += delta
		case ImportRowSucceeded:
			c.Succeeded += delta
		case ImportRowFailed:
			c.Failed += delta
		default:
			return fmt.Errorf("unknown import row status: %s", status)
		}
		return nil
	}
	if err := adjust(from, -1); err != nil {
		return ImportCounters{}, err
	}
	if err := adjust(to, 1); err != nil {
		return ImportCounters{}, err
	}
	if err := c.Validate(); err != nil {
		return ImportCounters{}, err
	}
	return c, nil
}

type ImportJob struct {
	ID           string          `json:"id"`
	Status       ImportJobStatus `json:"status"`
	Filename     string          `json:"filename"`
	ObjectKey    string          `json:"object_key,omitempty"`
	FileChecksum string          `json:"file_checksum,omitempty"`
	Counters     ImportCounters  `json:"counters"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ImportJobRow struct {
	JobID      string          `json:"job_id"`
	Ordinal    int64           `json:"ordinal"`
	RawPayload []byte          `json:"raw_payload,omitempty"`
	Checksum   string          `json:"checksum"`
	Status     ImportRowStatus `json:"status"`
	CustomerID string          `json:"customer_id,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
}
