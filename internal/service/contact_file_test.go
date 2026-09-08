package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contactFileRepositoryStub struct {
	fields []domain.ContactFileField
	calls  int
}

func (r *contactFileRepositoryStub) Fields(context.Context, string) ([]domain.ContactFileField, error) {
	r.calls++
	return append([]domain.ContactFileField(nil), r.fields...), nil
}
func (r *contactFileRepositoryStub) LoadProfiles(context.Context, string, []*domain.Contact) error {
	return nil
}

func TestContactFileSchemaPermissionsAndFreshFields(t *testing.T) {
	for _, resource := range []domain.PermissionResource{domain.PermissionResourceContacts, domain.PermissionResourceCustomers, domain.PermissionResourceLists} {
		t.Run(string(resource), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			auth := mocks.NewMockAuthService(ctrl)
			ctx := context.Background()
			membership := &domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{resource: {Read: true}}}
			auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws").Return(ctx, nil, membership, nil).AnyTimes()
			files := &contactFileRepositoryStub{fields: domain.BaseContactFileFields(nil)}
			s := &ImportJobService{auth: auth, fileFields: files}
			first, err := s.FileFields(ctx, "ws")
			if resource == domain.PermissionResourceLists {
				require.Error(t, err)
				assert.Zero(t, files.calls)
				return
			}
			require.NoError(t, err)
			files.fields = append(files.fields, domain.ContactFileField{Key: "profile.attributes.new", Header: "新字段", Type: "string"})
			next, err := s.FileFields(ctx, "ws")
			require.NoError(t, err)
			if resource == domain.PermissionResourceCustomers {
				assert.Len(t, next, len(first)+1)
			} else {
				assert.Len(t, next, len(first))
			}
		})
	}
}

func TestContactFileExportTemplateRoundTripThroughWorker(t *testing.T) {
	fields := domain.BaseContactFileFields(map[string]string{"custom_string_1": "会员编号", "custom_number_1": "积分"})
	fields = append(fields,
		domain.ContactFileField{Key: "profile.attributes.Credit.Score", Header: "评分 (profile.attributes.Credit.Score)", Type: "number"},
		domain.ContactFileField{Key: "profile.attributes.vip", Header: "VIP (profile.attributes.vip)", Type: "boolean"},
		domain.ContactFileField{Key: "profile.attributes.details", Header: "资料 (profile.attributes.details)", Type: "json"},
		domain.ContactFileField{Key: "profile.attributes.locked", Header: "只读 (profile.attributes.locked)", Type: "string", ReadOnly: true},
	)
	values := map[string]string{"email": "user@example.com", "external_id": "000123456789012345678", "first_name": "小明\n测试", "full_name": " 小明,测试 ", "phone": "+8613800000000", "language": "zh-CN", "timezone": "Asia/Shanghai", "custom_string_1": "00042", "custom_number_1": "0", "custom_json_1": `{"a":false}`, "profile.attributes.Credit.Score": "12.5", "profile.attributes.vip": "false", "profile.attributes.details": `{"line":"a\nb","items":[1,2]}`, "profile.attributes.locked": "must-ignore", "created_at": "2020-01-01T00:00:00Z"}
	headers, row := []string{}, []string{}
	for _, field := range fields {
		headers = append(headers, field.Header)
		row = append(row, values[field.Key])
	}
	var source bytes.Buffer
	source.WriteString("\uFEFF")
	writer := csv.NewWriter(&source)
	require.NoError(t, writer.Write(headers))
	require.NoError(t, writer.Write(row))
	writer.Flush()
	repo, customers := &importJobRepositoryMemory{}, &importCustomerServiceStub{}
	s, err := NewImportJobService(ImportJobServiceDependencies{Repository: repo, Customers: customers, FileFields: &contactFileRepositoryStub{fields: fields}, MaxRows: 100, ChunkSize: 10, MaxFileBytes: 1 << 20})
	require.NoError(t, err)
	job, err := s.StageCSV(context.Background(), "ws", "export.csv", nil, &source)
	require.NoError(t, err)
	require.Equal(t, int64(1), job.Counters.Pending)
	_, err = s.ProcessNextChunk(context.Background(), "ws", job.ID)
	require.NoError(t, err)
	require.Len(t, customers.requests, 1)
	input := customers.requests[0].Items[0].Customer
	assert.Equal(t, values["external_id"], *input.ExternalUserID)
	assert.Equal(t, "zh-CN", *input.Profile.Language)
	var attributes map[string]interface{}
	require.NoError(t, json.Unmarshal(input.Profile.Attributes.Merge, &attributes))
	assert.Equal(t, "小明\n测试", attributes["first_name"])
	assert.Equal(t, " 小明,测试 ", attributes["full_name"])
	assert.Equal(t, "00042", attributes["custom_string_1"])
	assert.Equal(t, float64(0), attributes["custom_number_1"])
	assert.Equal(t, float64(12.5), attributes["Credit.Score"])
	assert.Equal(t, false, attributes["vip"])
	assert.Equal(t, map[string]interface{}{"a": false}, attributes["custom_json_1"])
	assert.NotContains(t, attributes, "locked")
	assert.NotContains(t, attributes, "created_at")
}

func TestContactFileHeadersSurviveRenamesAndRejectAmbiguity(t *testing.T) {
	fields := domain.BaseContactFileFields(map[string]string{"custom_string_1": "新名称"})
	headers := []string{"email", "旧名称 (custom_string_1)"}
	require.True(t, resolveContactFileHeaders(headers, fields))
	assert.Equal(t, []string{"email", "custom_string_1"}, headers)
	fields = append(fields, domain.ContactFileField{Key: "profile.attributes.Score", Header: "评分 (profile.attributes.Score)"}, domain.ContactFileField{Key: "profile.attributes.score", Header: "评分 (profile.attributes.score)"})
	headers = []string{"email", "profile.attributes.Score", "profile.attributes.score"}
	require.True(t, resolveContactFileHeaders(headers, fields))
	assert.Equal(t, "profile.attributes.Score", headers[1])
	assert.False(t, resolveContactFileHeaders([]string{"email", "评分"}, fields))
}

func TestContactFileInvalidTypedValueIsAnExplicitFailedRow(t *testing.T) {
	fields := append(domain.BaseContactFileFields(nil), domain.ContactFileField{Key: "profile.attributes.score", Header: "score", Type: "number"})
	repo := &importJobRepositoryMemory{}
	s, err := NewImportJobService(ImportJobServiceDependencies{Repository: repo, FileFields: &contactFileRepositoryStub{fields: fields}, MaxRows: 100, ChunkSize: 10, MaxFileBytes: 1 << 20})
	require.NoError(t, err)
	job, err := s.StageCSV(context.Background(), "ws", "test.csv", nil, bytes.NewBufferString("email,score\nuser@example.com,invalid\n"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), job.Counters.Failed)
	assert.Equal(t, "mapping_error", repo.rows[1].ErrorCode)
}

func TestContactFileExportRequiresProfileReadPermission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		ctrl := gomock.NewController(t)
		auth := mocks.NewMockAuthService(ctrl)
		repo := mocks.NewMockContactRepository(ctrl)
		ctx := context.Background()
		member := &domain.UserWorkspace{Role: "member", Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}, domain.PermissionResourceCustomers: {Read: allowed}}}
		auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws").Return(ctx, nil, member, nil)
		request := &domain.GetContactsRequest{WorkspaceID: "ws", WithProfiles: true}
		if allowed {
			repo.EXPECT().GetContacts(ctx, request).Return(&domain.GetContactsResponse{Contacts: []*domain.Contact{{Email: "test@example.com"}}}, nil)
		}
		s := &ContactService{repo: repo, authService: auth, fileFields: &contactFileRepositoryStub{}}
		result, err := s.GetContacts(ctx, request)
		if allowed {
			require.NoError(t, err)
			require.Len(t, result.Contacts, 1)
		} else {
			require.Error(t, err)
			assert.Nil(t, result)
		}
	}
}
