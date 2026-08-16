package personal_setting

import (
	"context"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_settingv1 "chatbasket-api/gen/proto/personal/personal_setting"
	rpc_personal_settingv1connect "chatbasket-api/gen/proto/personal/personal_setting/rpc_personal_settingv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"

	"connectrpc.com/connect"
)

type settingConnectServer struct {
	settingService     *settingService
	personalSseManager *personal_sse.Manager
}

func newSettingConnectServer(service *settingService, personalSseManager *personal_sse.Manager) rpc_personal_settingv1connect.SettingServiceHandler {
	return &settingConnectServer{
		settingService:     service,
		personalSseManager: personalSseManager,
	}
}

func (s *settingConnectServer) SetCentralDevice(ctx context.Context, req *connect.Request[rpc_personal_settingv1.SetCentralDeviceRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionToken, err := kit.GetConnectRpcSessionID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res, err := s.settingService.setCentralDevice(ctx, userID.UuidUserId, sessionToken)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if s.personalSseManager != nil {
		s.personalSseManager.UnregisterUserConnections(userID.UuidUserId)
	}

	return connect.NewResponse(res), nil
}

func (s *settingConnectServer) UpdateSessionNotificationToken(ctx context.Context, req *connect.Request[rpc_personal_settingv1.RegisterOrUpdateNotificationTokenRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionToken, err := kit.GetConnectRpcSessionID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &registerOrUpdateFcmOrApnTokenPayload{
		Token:      req.Msg.Token,
		Type:       req.Msg.Type,
		DeviceName: req.Msg.DeviceName,
	}

	res, err := s.settingService.updateSessionNotificationToken(ctx, userID.UuidUserId, sessionToken, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}
