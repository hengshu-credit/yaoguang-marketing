package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

type themeTestStore struct {
	calls int
	id    string
	err   error
}

func (s *themeTestStore) Set(_ context.Context, id string, _ *domain.ConsoleThemeSettings) error {
	s.calls++
	s.id = id
	return s.err
}
func TestConsoleThemeOwnerScopeAndFailure(t *testing.T) {
	store := &themeTestStore{}
	for _, role := range []string{"member", "owner"} {
		s := NewConsoleThemeService(store, businessTestAuth{member: &domain.UserWorkspace{Role: role, Permissions: domain.NewFullPermissions()}})
		value := domain.DefaultConsoleTheme()
		_, err := s.Save(context.Background(), "workspace-a", &value)
		if role == "member" {
			require.Error(t, err)
			require.Zero(t, store.calls)
		} else {
			require.NoError(t, err)
			require.Equal(t, "workspace-a", store.id)
		}
	}
	s := NewConsoleThemeService(store, businessTestAuth{member: &domain.UserWorkspace{Role: "owner"}})
	value := domain.DefaultConsoleTheme()
	value.PrimaryColor = "bad"
	_, err := s.Save(context.Background(), "workspace-a", &value)
	require.Error(t, err)
	require.Equal(t, 1, store.calls)
	store.err = errors.New("write unavailable")
	value = domain.DefaultConsoleTheme()
	_, err = s.Save(context.Background(), "workspace-a", &value)
	require.ErrorIs(t, err, store.err)
}
