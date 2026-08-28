package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// When ROOT_EMAIL holds a list, the setup wizard must create the primary (first)
// email as the root user, not a user whose email is the raw comma-joined string.
// The full list is still persisted so InitializeDatabase can create the rest on
// startup.
func TestSetupService_Initialize_UsesPrimaryRootEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingRepo := NewMockSettingRepository()
	settingService := NewSettingService(settingRepo)
	userRepo := domainmocks.NewMockUserRepository(ctrl)

	envConfig := &EnvironmentConfig{RootEmail: "primary@example.com,second@example.com"}

	setupService := NewSetupService(
		settingService,
		&UserService{},
		userRepo,
		logger.NewLogger(),
		"test-secret-key",
		nil,
		envConfig,
	)

	var createdUser *domain.User
	userRepo.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *domain.User) error {
			createdUser = u
			return nil
		})

	// SMTP is not env-configured, so it must be provided to pass validation.
	cfg := &SetupConfig{
		APIEndpoint:   "https://api.example.com",
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPFromEmail: "noreply@example.com",
	}

	err := setupService.Initialize(context.Background(), cfg)
	require.NoError(t, err)

	require.NotNil(t, createdUser)
	assert.Equal(t, "primary@example.com", createdUser.Email, "root user should use the primary email, not the raw list")

	stored, err := settingRepo.Get(context.Background(), "root_email")
	require.NoError(t, err)
	assert.Equal(t, "primary@example.com,second@example.com", stored.Value, "the full root email list should be persisted")
}
