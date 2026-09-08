package service

import (
	"bufio"
	"bytes"
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
	Tasks        ImportTaskScheduler
	Auth         domain.AuthService
	MaxRows      int
	ChunkSize    int
	MaxFileBytes int64
	FileFields   domain.ContactFileRepository
}

type ImportTaskScheduler interface {
	CreateTask(context.Context, string, *domain.Task) error
}

type ImportJobService struct {
	repository domain.ImportJobRepository
	customers  domain.CustomerService
	maxRows    int
	chunkSize  int
	maxBytes   int64
	now        func() time.Time
	auth       domain.AuthService
	tasks      ImportTaskScheduler
	fileFields domain.ContactFileRepository
}

func NewImportJobService(dependencies ImportJobServiceDependencies) (*ImportJobService, error) {
	if dependencies.Repository == nil || dependencies.MaxRows <= 0 || dependencies.ChunkSize <= 0 || dependencies.MaxFileBytes <= 0 {
		return nil, errors.New("import repository and positive limits are required")
	}
	return &ImportJobService{repository: dependencies.Repository, customers: dependencies.Customers,
		maxRows: dependencies.MaxRows, chunkSize: dependencies.ChunkSize, maxBytes: dependencies.MaxFileBytes, now: time.Now, auth: dependencies.Auth, tasks: dependencies.Tasks, fileFields: dependencies.FileFields}, nil
}

func (s *ImportJobService) SetTaskScheduler(tasks ImportTaskScheduler) {
	s.tasks = tasks
}

func (s *ImportJobService) authorize(ctx context.Context, workspaceID string, permission domain.PermissionType) (context.Context, error) {
	if s.auth == nil {
		return ctx, nil
	}
	authorized, _, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, err
	}
	if membership == nil || !membership.HasPermission(domain.PermissionResourceCustomers, permission) {
		return ctx, domain.NewPermissionError(domain.PermissionResourceCustomers, permission, "Insufficient permissions")
	}
	return authorized, nil
}

// StageCSV persists every CSV data record before a worker may process it.
// Malformed rows are persisted as explicit failures instead of disappearing.
func (s *ImportJobService) StageCSV(ctx context.Context, workspaceID, filename string, listIDs []string, source io.Reader) (*domain.ImportJob, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(filename) == "" || source == nil {
		return nil, errors.New("workspace, filename and source are required")
	}
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return nil, err
	}
	ctx = authorized
	fileFields, err := s.loadFileFields(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	normalizedListIDs, err := normalizeImportListIDs(listIDs)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	job := domain.ImportJob{ID: uuid.New().String(), Status: domain.ImportJobUploading, Filename: filename,
		ListIDs: normalizedListIDs, Counters: domain.ImportCounters{}, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateImportJob(ctx, workspaceID, job); err != nil {
		return nil, err
	}

	hash := sha256.New()
	buffered := bufio.NewReader(io.TeeReader(source, hash))
	var bomBytes int64
	if prefix, _ := buffered.Peek(3); bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = buffered.Discard(3)
		bomBytes = 3
	}
	reader := csv.NewReader(buffered)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	var headers []string
	var ordinal int64
	headerRead := false
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
	for {
		fields, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if !headerRead {
			headerRead = true
			headers = fields
			if readErr != nil || !resolveContactFileHeaders(headers, fileFields) {
				rejectedReason = "CSV 表头无效"
			}
			if readErr != nil {
				break
			}
			continue
		}
		var encoded bytes.Buffer
		writer := csv.NewWriter(&encoded)
		_ = writer.Write(fields)
		writer.Flush()
		line := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
		ordinal++
		payload, errorCode := importCSVPayload(headers, line)
		if readErr != nil {
			errorCode = "csv_parse_error"
		}
		if errorCode == "" {
			normalized, err := normalizeContactFilePayload(payload, fileFields)
			if err != nil {
				errorCode = "mapping_error"
			} else {
				payload = normalized
			}
		}
		status := domain.ImportRowPending
		if errorCode != "" {
			status = domain.ImportRowFailed
		}
		if ordinal > int64(s.maxRows) {
			status, errorCode = domain.ImportRowFailed, "row_limit_exceeded"
			rejectedReason = fmt.Sprintf("导入行数超过后台限制 %d", s.maxRows)
		}
		if reader.InputOffset()+bomBytes > s.maxBytes {
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
		if readErr != nil {
			var parseErr *csv.ParseError
			if !errors.As(readErr, &parseErr) {
				rejectedReason = "读取上传流失败: " + readErr.Error()
				break
			}
		}
	}
	if len(headers) == 0 || ordinal == 0 {
		rejectedReason = "导入文件必须包含表头和至少一条客户数据"
	}
	if err := flush(); err != nil {
		return nil, err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	if rejectedReason != "" {
		if err := s.repository.RejectImportJob(ctx, workspaceID, job.ID, rejectedReason); err != nil {
			return nil, err
		}
	} else {
		if err := s.repository.CommitImportJob(ctx, workspaceID, job.ID, checksum); err != nil {
			return nil, err
		}
		committed, err := s.repository.GetImportJob(ctx, workspaceID, job.ID)
		if err != nil {
			return nil, err
		}
		if committed.Status == domain.ImportJobStaged && s.tasks != nil {
			task := &domain.Task{ID: job.ID, WorkspaceID: workspaceID, Type: domain.ImportCustomersTaskType,
				Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 20, RetryInterval: 15,
				State: &domain.TaskState{ImportCustomers: &domain.ImportCustomersState{JobID: job.ID, TotalRows: committed.Counters.Total}}}
			if err := s.tasks.CreateTask(ctx, workspaceID, task); err != nil {
				return nil, fmt.Errorf("create import processing task: %w", err)
			}
		}
	}
	return s.repository.GetImportJob(ctx, workspaceID, job.ID)
}

func normalizeImportListIDs(listIDs []string) ([]string, error) {
	if len(listIDs) == 0 {
		return nil, nil
	}
	memberships := make([]domain.CustomerListMembershipInput, 0, len(listIDs))
	seen := make(map[string]struct{}, len(listIDs))
	for _, rawListID := range listIDs {
		listID := strings.TrimSpace(rawListID)
		if _, exists := seen[listID]; exists {
			continue
		}
		seen[listID] = struct{}{}
		memberships = append(memberships, domain.CustomerListMembershipInput{ListID: listID, Status: "active"})
	}
	placeholder := "import-list-validation"
	input := domain.CustomerUpsertInput{ExternalUserID: &placeholder, ListMembershipsAdd: &memberships}
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid import list selection: %w", err)
	}
	result := make([]string, len(memberships))
	for index, membership := range memberships {
		result[index] = membership.ListID
	}
	return result, nil
}

func parseCSVLine(line []byte) []string {
	reader := csv.NewReader(strings.NewReader(string(line)))
	reader.TrimLeadingSpace = true
	fields, err := reader.Read()
	if err != nil {
		return nil
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
	return s.processNextChunk(ctx, workspaceID, jobID, true)
}

// ProcessNextChunkInternal is used only by the registered durable task
// processor. The scheduler has already selected the workspace-scoped task, so
// it must not depend on request JWT state that does not exist after restart.
func (s *ImportJobService) ProcessNextChunkInternal(ctx context.Context, workspaceID, jobID string) (int, error) {
	return s.processNextChunk(ctx, workspaceID, jobID, false)
}

func (s *ImportJobService) processNextChunk(ctx context.Context, workspaceID, jobID string, checkAuth bool) (int, error) {
	if s.customers == nil {
		return 0, errors.New("customer service is required for import processing")
	}
	if checkAuth {
		authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
		if err != nil {
			return 0, err
		}
		ctx = authorized
	}
	job, err := s.repository.GetImportJob(ctx, workspaceID, jobID)
	if err != nil {
		return 0, err
	}
	rows, claimToken, err := s.repository.ClaimImportRows(ctx, workspaceID, jobID, s.chunkSize, 2*time.Minute)
	if err != nil || len(rows) == 0 {
		return len(rows), err
	}
	items := make([]domain.CustomerBatchUpsertItem, 0, len(rows))
	validRows := make([]domain.ImportJobRow, 0, len(rows))
	for _, row := range rows {
		item, parseErr := importRowToCustomer(row, job.ListIDs)
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
			job, getErr := s.repository.GetImportJob(ctx, workspaceID, jobID)
			if getErr == nil && job.Status == domain.ImportJobCancelled {
				return len(rows), nil
			}
			return index, err
		}
	}
	return len(rows), nil
}

func (s *ImportJobService) Get(ctx context.Context, workspaceID, jobID string) (*domain.ImportJob, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	return s.repository.GetImportJob(authorized, workspaceID, jobID)
}

func (s *ImportJobService) GetInternal(ctx context.Context, workspaceID, jobID string) (*domain.ImportJob, error) {
	return s.repository.GetImportJob(ctx, workspaceID, jobID)
}

func (s *ImportJobService) List(ctx context.Context, workspaceID string, limit, offset int) (*domain.ImportJobList, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repository.ListImportJobs(authorized, workspaceID, limit, offset)
	if err != nil {
		return nil, err
	}
	return &domain.ImportJobList{Items: items, Total: total, Limit: normalizedImportLimit(limit), Offset: max(offset, 0)}, nil
}

func normalizedImportLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func (s *ImportJobService) Cancel(ctx context.Context, workspaceID, jobID string) error {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeWrite)
	if err != nil {
		return err
	}
	return s.repository.CancelImportJob(authorized, workspaceID, jobID, "用户取消导入；未完成行已记录为明确失败")
}

func (s *ImportJobService) Errors(ctx context.Context, workspaceID, jobID string, limit, offset int) ([]domain.ImportJobError, int, error) {
	authorized, err := s.authorize(ctx, workspaceID, domain.PermissionTypeRead)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repository.ListImportJobErrors(authorized, workspaceID, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.ImportJobError, 0, len(rows))
	for _, row := range rows {
		values := map[string]string{}
		_ = json.Unmarshal(row.RawPayload, &values)
		external := strings.TrimSpace(values["external_id"])
		if external == "" {
			external = strings.TrimSpace(values["external_user_id"])
		}
		result = append(result, domain.ImportJobError{Ordinal: row.Ordinal,
			ExternalUserID: external, DisplayIdentity: maskedImportIdentity(values),
			ErrorCode: row.ErrorCode, ErrorDetail: row.ErrorDetail})
	}
	return result, total, nil
}

func maskedImportIdentity(values map[string]string) string {
	if email := strings.TrimSpace(values["email"]); email != "" {
		parts := strings.SplitN(email, "@", 2)
		if len(parts) == 2 {
			prefix := parts[0]
			if len(prefix) > 2 {
				prefix = prefix[:2]
			}
			return prefix + "***@" + parts[1]
		}
	}
	if phone := strings.TrimSpace(values["phone"]); phone != "" {
		if len(phone) <= 4 {
			return "****"
		}
		return "***" + phone[len(phone)-4:]
	}
	return ""
}

func importRowToCustomer(row domain.ImportJobRow, listIDs []string) (domain.CustomerBatchUpsertItem, error) {
	values := map[string]string{}
	if err := json.Unmarshal(row.RawPayload, &values); err != nil {
		return domain.CustomerBatchUpsertItem{}, err
	}
	input := domain.CustomerUpsertInput{}
	external := strings.TrimSpace(values["external_id"])
	if external == "" {
		external = strings.TrimSpace(values["external_user_id"])
	}
	if external != "" {
		input.ExternalUserID = &external
	}
	if email := strings.TrimSpace(values["email"]); email != "" {
		input.Identities = append(input.Identities, domain.CustomerIdentityInput{Type: domain.CustomerIdentityEmail, Value: email, Primary: true})
	}
	if phone := strings.TrimSpace(values["phone"]); phone != "" {
		input.Identities = append(input.Identities, domain.CustomerIdentityInput{Type: domain.CustomerIdentityPhone, Value: phone, Primary: true})
	}
	profile := &domain.CustomerProfilePatch{}
	attributes := map[string]interface{}{}
	for _, field := range domain.BaseContactFileFields(nil) {
		raw := values[field.Key]
		if raw == "" || field.ReadOnly || field.Key == "email" || field.Key == "external_id" {
			continue
		}
		switch field.Key {
		case "language":
			profile.Language = &raw
		case "timezone":
			profile.Timezone = &raw
		default:
			value, err := parseContactFileValue(raw, field.Type)
			if err != nil {
				return domain.CustomerBatchUpsertItem{}, err
			}
			attributes[field.Key] = value
		}
	}
	if raw := values["profile.status"]; raw != "" {
		profile.Status = &raw
	}
	if raw := values["profile.tags"]; raw != "" {
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			return domain.CustomerBatchUpsertItem{}, err
		}
		input.Tags = &tags
	}
	if raw := values["profile.attributes"]; raw != "" {
		var dynamic map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &dynamic); err != nil {
			return domain.CustomerBatchUpsertItem{}, err
		}
		for key, value := range dynamic {
			attributes[key] = value
		}
	}
	if len(attributes) > 0 {
		encoded, err := json.Marshal(attributes)
		if err != nil {
			return domain.CustomerBatchUpsertItem{}, err
		}
		profile.Attributes = &domain.CustomerAttributesPatch{Merge: encoded}
	}
	if profile.Language != nil || profile.Timezone != nil || profile.Status != nil || profile.Attributes != nil {
		input.Profile = profile
	}
	if len(listIDs) > 0 {
		memberships := make([]domain.CustomerListMembershipInput, len(listIDs))
		for index, listID := range listIDs {
			memberships[index] = domain.CustomerListMembershipInput{ListID: listID, Status: "active"}
		}
		input.ListMembershipsAdd = &memberships
	}
	if err := input.Validate(); err != nil {
		return domain.CustomerBatchUpsertItem{}, err
	}
	return domain.CustomerBatchUpsertItem{IdempotencyKey: fmt.Sprintf("import:%s:%d:%s", row.JobID, row.Ordinal, row.Checksum), Customer: input}, nil
}
