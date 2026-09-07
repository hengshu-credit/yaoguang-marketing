package service

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"testing"
	"time"
)

func TestFunnelAsyncQueryRejectsMissingPermissionBeforeSaving(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "w").Return(context.Background(), &domain.User{}, &domain.UserWorkspace{Role: "member"}, nil)
	s := NewFunnelService(nil, nil, auth)
	_, err := s.StartQuery(context.Background(), domain.FunnelQuery{WorkspaceID: "w", Start: time.Now()})
	if err == nil {
		t.Fatal("async query must not bypass authorization")
	}
}
