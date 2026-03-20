package personal_setting

import (
	"chatbasket-apinext/internal/modules/core/core_auth"
	"chatbasket-apinext/internal/platform/kit"
	"context"

	"github.com/google/uuid"
)

// authSettingProvider defines the minimal set of methods required from the Auth module.
type authSettingProvider interface {
	SetCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*kit.StatusOkay, error)
	RegisterOrUpdateFcmOrApnToken(ctx context.Context, payload *core_auth.RegisterOrUpdateFcmOrApnTokenPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, error)
}

// settingService manages personal-mode settings by delegating to specialized modules.
type settingService struct {
	authProvider authSettingProvider
}

// NewSettingService creates a new settingService instance.
func NewSettingService(authProvider authSettingProvider) *settingService {
	return &settingService{
		authProvider: authProvider,
	}
}

// setCentralDevice promotes the current session to be the user's primary device.
func (s *settingService) setCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*kit.StatusOkay, error) {
	ok, err := s.authProvider.SetCentralDevice(ctx, userID, token)
	if err != nil {
		return nil, err
	}
	return ok, nil
}

// updateSessionNotificationToken updates the push notification token for the current session.
func (s *settingService) updateSessionNotificationToken(ctx context.Context, userID uuid.UUID, sessionToken string, payload *registerOrUpdateFcmOrApnTokenPayload) (*kit.StatusOkay, error) {
	// Map local model to Auth model for delegation
	authPayload := &core_auth.RegisterOrUpdateFcmOrApnTokenPayload{
		Token:      payload.Token,
		Type:       payload.Type,
		DeviceName: payload.DeviceName,
	}

	ok, err := s.authProvider.RegisterOrUpdateFcmOrApnToken(ctx, authPayload, userID, sessionToken)
	if err != nil {
		return nil, err
	}

	return ok, nil
}
