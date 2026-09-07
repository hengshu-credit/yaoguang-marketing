package service

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"testing"
)

func TestFunnelServiceRejectsUnauthorizedBeforeReading(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "w").Return(context.Background(), &domain.User{}, &domain.UserWorkspace{Role: "member"}, nil)
	s := NewFunnelService(nil, nil, auth)
	_, err := s.Query(context.Background(), domain.FunnelQuery{WorkspaceID: "w"})
	if err == nil {
		t.Fatal("analytics permission required")
	}
}
func TestSubjectStateServiceRejectsUnauthorizedManualWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "w").Return(context.Background(), &domain.User{}, &domain.UserWorkspace{Role: "member"}, nil)
	s := NewSubjectStateService(nil, nil, nil, auth)
	err := s.Set(context.Background(), SubjectStateSetRequest{WorkspaceID: "w"})
	if err == nil {
		t.Fatal("customer write permission required")
	}
}
