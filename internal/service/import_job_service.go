package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type ImportJobServiceDependencies struct {
	Repository   domain.ImportJobRepository
	Customers    domain.CustomerService
	MaxRows      int
	ChunkSize    int
	MaxFileBytes int64
}

type ImportJobService struct {
	repository domain.ImportJobRepository
	customers  domain.CustomerService
	maxRows    int
	chunkSize  int
	maxBytes   int64
	now        func() time.Time
}

func NewImportJobService(dependencies ImportJobServiceDependencies) (*ImportJobService, error) {
	if dependencies.Repository == nil || dependencies.MaxRows <= 0 || dependencies.ChunkSize <= 0 || dependencies.MaxFileBytes <= 0 {
		return nil, errors.New("import repository and positive limits are required")
	}
	return &ImportJobService{repository: dependencies.Repository, customers: dependencies.Customers,
		maxRows: dependencies.MaxRows, chunkSize: dependencies.ChunkSize, maxBytes: dependencies.MaxFileBytes, now: time.Now}, nil
}

// StageCSV persists every physical data row before a worker may process it.
// Malformed rows are persisted as explicit failures instead of disappearing.
func (s *ImportJobService) StageCSV(ctx context.Context, workspaceID, filename string, source io.Reader) (*domain.ImportJob, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(filename) == "" || source == nil {
		return nil, errors.New("workspace, filename and source are required")
	}
	now := s.now().UTC()
	job := domain.ImportJob{ID: uuid.New().String(), Status: domain.ImportJobUploading, Filename: filename,
		Counters: domain.ImportCounters{}, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateImportJob(ctx, workspaceID, job); err != nil {
		return nil, err
	}

	hash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(source, hash))
	maxScannerCapacity := int(s.maxBytes)
	if maxScannerCapacity > 16<<20 {
		maxScannerCapacity = 16 << 20
	}
	scanner.Buffer(make([]byte, 64*1024), maxScannerCapacity)
	var headers []string
	var ordinal, fileBytes int64
	rows := make([]domain.ImportJobRow, 0, s.chunkSize)
	rejectedReason := ""
	flush := func() error {
		if len(rows) == 0 {
			return nil
		}
		_, err := s.repository.StageImportRows(ctx, workspaceID, job.ID, rows)
		rows = rows[:0]
		return err
	}
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		fileBytes += int64(len(line)) + 1
		if headers == nil {
			headers = parseCSVLine(line)
			if len(headers) == 0 {
				rejectedReason = "CSV 表头无效"
			}
			continue
		}
		ordinal++
		payload, errorCode := importCSVPayload(headers, line)
		status := domain.ImportRowPending
		if errorCode != "" {
			status = domain.ImportRowFailed
		}
		if ordinal > int64(s.maxRows) {
			status, errorCode = domain.ImportRowFailed, "row_limit_exceeded"
			rejectedReason = fmt.Sprintf("导入行数超过后台限制 %d", s.maxRows)
		}
		if fileBytes > s.maxBytes {
			status, errorCode = domain.ImportRowFailed, "file_size_exceeded"
			rejectedReason = fmt.Sprintf("导入文件超过后台限制 %d bytes", s.maxBytes)
		}
		digest := sha256.Sum256(line)
		rows = append(rows, domain.ImportJobRow{JobID: job.ID, Ordinal: ordinal, RawPayload: payload,
			Checksum: hex.EncodeToString(digest[:]), Status: status, ErrorCode: errorCode})
		if len(rows) == s.chunkSize {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		rejectedReason = "读取上传流失败: " + err.Error()
	}
	if err := flush(); err != nil {
		return nil, err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	if rejectedReason != "" {
		if err := s.repository.RejectImportJob(ctx, workspaceID, job.ID, rejectedReason); err != nil {
			return nil, err
		}
	} else if err := s.repository.CommitImportJob(ctx, workspaceID, job.ID, checksum); err != nil {
		return nil, err
	}
	return s.repository.GetImportJob(ctx, workspaceID, job.ID)
}

func parseCSVLine(line []byte) []string {
	reader := csv.NewReader(strings.NewReader(string(line)))
	reader.TrimLeadingSpace = true
	fields, err := reader.Read()
	if err != nil {
		return nil
	}
	for index := range fields {
		fields[index] = strings.TrimSpace(fields[index])
		if fields[index] == "" {
			return nil
		}
	}
	return fields
}

func importCSVPayload(headers []string, line []byte) ([]byte, string) {
	fields := parseCSVLine(line)
	if len(fields) != len(headers) {
		payload, _ := json.Marshal(map[string]string{"_raw": string(line)})
		return payload, "csv_parse_error"
	}
	payload := make(map[string]string, len(headers))
	for index := range headers {
		payload[headers[index]] = fields[index]
	}
	encoded, _ := json.Marshal(payload)
	return encoded, ""
}

// ProcessNextChunk claims a bounded chunk and persists one terminal result per
// row. The idempotency key makes lease-expiry replay safe.
func (s *ImportJobService) ProcessNextChunk(ctx context.Context, workspaceID, jobID string) (int, error) {
	if s.customers == nil {
		return 0, errors.New("customer service is required for import processing")
	}
	rows, claimToken, err := s.repository.ClaimImportRows(ctx, workspaceID, jobID, s.chunkSize, 2*time.Minute)
	if err != nil || len(rows) == 0 {
		return len(rows), err
	}
	items := make([]domain.CustomerBatchUpsertItem, 0, len(rows))
	validRows := make([]domain.ImportJobRow, 0, len(rows))
	for _, row := range rows {
		item, parseErr := importRowToCustomer(row)
		if parseErr != nil {
			if completeErr := s.repository.CompleteImportRow(ctx, workspaceID, jobID, row.Ordinal, claimToken, domain.ImportRowFailed, "", "", "mapping_error"); completeErr != nil {
				return 0, completeErr
			}
			continue
		}
		items, validRows = append(items, item), append(validRows, row)
	}
	if len(items) == 0 {
		return len(rows), nil
	}
	response, err := s.customers.UpsertCustomerBatch(ctx, &domain.CustomerBatchUpsertRequest{WorkspaceID: workspaceID, Items: items})
	if err != nil {
		return 0, err
	}
	if len(response.Results) != len(validRows) {
		return 0, errors.New("customer batch result count does not match claimed import rows")
	}
	for index, result := range response.Results {
		status, customerID, action, errorCode := domain.ImportRowSucceeded, "", "", ""
		if result.Status == "error" || result.Customer == nil {
			status = domain.ImportRowFailed
			if result.Error != nil {
				errorCode = result.Error.Code
			}
		} else {
			customerID, action = result.Customer.CustomerID, result.Customer.Action
		}
		if err := s.repository.CompleteImportRow(ctx, workspaceID, jobID, validRows[index].Ordinal, claimToken, status, customerID, action, errorCode); err != nil {
			return index, err
		}
	}
	return len(rows), nil
}

func importRowToCustomer(row domain.ImportJobRow) (domain.CustomerBatchUpsertItem, error) {
	values := map[string]string{}
	if err := json.Unmarshal(row.RawPayload, &values); err != nil {
		return domain.CustomerBatchUpsertItem{}, err
	}
	input := domain.CustomerUpsertInput{}
	if external := strings.TrimSpace(values["external_user_id"]); external != "" {
		input.ExternalUserID = &external
	}
	if email := strings.TrimSpace(values["email"]); email != "" {
		input.Identities = append(input.Identities, domain.CustomerIdentityInput{Type: domain.CustomerIdentityEmail, Value: email, Primary: true})
	}
	if phone := strings.TrimSpace(values["phone"]); phone != "" {
		input.Identities = append(input.Identities, domain.CustomerIdentityInput{Type: domain.CustomerIdentityPhone, Value: phone, Primary: true})
	}
	if err := input.Validate(); err != nil {
		return domain.CustomerBatchUpsertItem{}, err
	}
	return domain.CustomerBatchUpsertItem{IdempotencyKey: fmt.Sprintf("import:%s:%d:%s", row.JobID, row.Ordinal, row.Checksum), Customer: input}, nil
}
