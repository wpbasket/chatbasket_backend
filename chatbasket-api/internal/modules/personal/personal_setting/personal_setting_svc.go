package personal_setting

import (
	"context"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
)

// coreAuthPersonalSettingProvider defines the minimal set of methods required from the Auth module.
type coreAuthPersonalSettingProvider interface {
	SetCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*kit.StatusOkay, error)
	RegisterOrUpdateFcmOrApnToken(ctx context.Context, payload *core_auth.RegisterOrUpdateFcmOrApnTokenPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, error)
}

// settingService manages personal-mode settings by delegating to specialized modules.
type settingService struct {
	coreAuthPersonalSettingProvider coreAuthPersonalSettingProvider
}

// NewSettingService creates a new settingService instance.
func NewSettingService(coreAuthPersonalSettingProvider coreAuthPersonalSettingProvider) *settingService {
	return &settingService{
		coreAuthPersonalSettingProvider: coreAuthPersonalSettingProvider,
	}
}

// setCentralDevice promotes the current session to be the user's primary device.
func (s *settingService) setCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*rpc_common_modelv1.StatusOkay, error) {
	ok, err := s.coreAuthPersonalSettingProvider.SetCentralDevice(ctx, userID, token)
	if err != nil {
		return nil, err
	}
	return &rpc_common_modelv1.StatusOkay{
		Status:  ok.Status,
		Message: ok.Message,
	}, nil
}

// updateSessionNotificationToken updates the push notification token for the current session.
func (s *settingService) updateSessionNotificationToken(ctx context.Context, userID uuid.UUID, sessionToken string, payload *registerOrUpdateFcmOrApnTokenPayload) (*rpc_common_modelv1.StatusOkay, error) {
	// Map local model to Auth model for delegation
	authPayload := &core_auth.RegisterOrUpdateFcmOrApnTokenPayload{
		Token:      payload.Token,
		Type:       payload.Type,
		DeviceName: payload.DeviceName,
	}

	ok, err := s.coreAuthPersonalSettingProvider.RegisterOrUpdateFcmOrApnToken(ctx, authPayload, userID, sessionToken)
	if err != nil {
		return nil, err
	}

	return &rpc_common_modelv1.StatusOkay{
		Status:  ok.Status,
		Message: ok.Message,
	}, nil
}

