package personal_profile

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"chatbasket-api/internal/platform/kit"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_profilev1 "chatbasket-api/gen/proto/personal/personal_profile"
	rpc_personal_profilev1connect "chatbasket-api/gen/proto/personal/personal_profile/rpc_personal_profilev1connect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

type profileConnectServer struct {
	profileService *profileService
}

func newProfileConnectServer(service *profileService) rpc_personal_profilev1connect.ProfileServiceHandler {
	return &profileConnectServer{
		profileService: service,
	}
}



func (s *profileConnectServer) CreateUserProfile(ctx context.Context, req *connect.Request[rpc_personal_profilev1.CreateUserProfileRequest]) (*connect.Response[rpc_personal_profilev1.CreateUserProfileResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.NewConnectRpcError(401, "unauthorized", "User id is missing or invalid")
	}
	email, err := kit.GetConnectRpcEmail(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidEmailContext)
	}

	payload := &createUserProfilePayload{
		Name:        strings.TrimSpace(req.Msg.Name),
		ProfileType: req.Msg.ProfileType,
	}

	user, err := s.profileService.CreateUserProfile(ctx, payload, &userID, email)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_personal_profilev1.CreateUserProfileResponse{
		User: toProtoPrivateUser(user),
	}), nil
}

func (s *profileConnectServer) GetProfile(ctx context.Context, req *connect.Request[rpc_personal_profilev1.GetProfileRequest]) (*connect.Response[rpc_personal_profilev1.GetProfileResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.NewConnectRpcError(401, "unauthorized", "User id is missing or invalid")
	}
	email, err := kit.GetConnectRpcEmail(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidEmailContext)
	}

	user, err := s.profileService.GetProfile(ctx, &userID, email)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_personal_profilev1.GetProfileResponse{
		User: toProtoPrivateUser(user),
	}), nil
}

func (s *profileConnectServer) UpdateUserProfile(ctx context.Context, req *connect.Request[rpc_personal_profilev1.UpdateUserProfileRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	var name, bio, profileType *string
	if req.Msg.Name != nil {
		n := strings.TrimSpace(*req.Msg.Name)
		name = &n
	}
	if req.Msg.Bio != nil {
		b := strings.TrimSpace(*req.Msg.Bio)
		bio = &b
	}
	if req.Msg.ProfileType != nil {
		pt := *req.Msg.ProfileType
		profileType = &pt
	}

	payload := &updateUserProfilePayload{
		Name:        name,
		Bio:         bio,
		ProfileType: profileType,
	}

	status, err := s.profileService.UpdateUserProfile(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{
		Status:  status.Status,
		Message: status.Message,
	}), nil
}

func (s *profileConnectServer) PresignAvatar(ctx context.Context, req *connect.Request[rpc_personal_profilev1.PresignAvatarRequest]) (*connect.Response[rpc_personal_profilev1.PresignAvatarResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	res, err := s.profileService.PresignAvatarUpload(ctx, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_personal_profilev1.PresignAvatarResponse{
		FileId:       res.FileID,
		PresignedUrl: res.PresignedURL,
	}), nil
}

func (s *profileConnectServer) ConfirmAvatar(ctx context.Context, req *connect.Request[rpc_personal_profilev1.ConfirmAvatarRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.FileId == "" {
		return nil, kit.NewConnectRpcError(400, "bad_request", "file_id is required")
	}

	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	status, err := s.profileService.ConfirmAvatarUpload(ctx, userID, req.Msg.FileId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{
		Status:  status.Status,
		Message: status.Message,
	}), nil
}

func (s *profileConnectServer) RemoveProfilePicture(ctx context.Context, req *connect.Request[rpc_personal_profilev1.RemoveProfilePictureRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	status, err := s.profileService.RemoveUserProfilePicture(ctx, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{
		Status:  status.Status,
		Message: status.Message,
	}), nil
}

func (s *profileConnectServer) UploadE2EEPublicKey(ctx context.Context, req *connect.Request[rpc_personal_profilev1.UploadE2EEPublicKeyRequest]) (*connect.Response[rpc_personal_profilev1.UploadE2EEPublicKeyResponse], error) {
	key := req.Msg.E2EePublicKey
	if key == "" {
		return nil, kit.NewConnectRpcError(http.StatusBadRequest, "bad_request", "e2ee_public_key is required")
	}

	if len(key) != 44 {
		return nil, kit.NewConnectRpcError(http.StatusBadRequest, "bad_request", "e2ee_public_key must be exactly 44 characters (Base64 X25519)")
	}

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return nil, kit.NewConnectRpcError(http.StatusBadRequest, "bad_request", "e2ee_public_key must be a valid base64-encoded 32-byte X25519 public key")
	}

	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	sessionUUIDVal, err := kit.GetConnectRpcSessionUUID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidSessionContext)
	}

	res, err := s.profileService.SaveE2EEPublicKey(ctx, userID, sessionUUIDVal, key)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_personal_profilev1.UploadE2EEPublicKeyResponse{
		Status:       res.Status,
		Message:      res.Message,
		KeysRevision: res.KeysRevision,
	}), nil
}

func (s *profileConnectServer) GetE2EEPublicKey(ctx context.Context, req *connect.Request[rpc_personal_profilev1.GetE2EEPublicKeyRequest]) (*connect.Response[rpc_personal_profilev1.GetE2EEPublicKeyResponse], error) {
	if req.Msg.UserId == "" {
		return nil, kit.NewConnectRpcError(http.StatusBadRequest, "bad_request", "user_id is required")
	}

	uuidVal, err := kit.StringToUUID(req.Msg.UserId)
	if err != nil {
		return nil, kit.NewConnectRpcError(http.StatusBadRequest, "bad_request", "invalid user_id format")
	}

	var callerSessionID *uuid.UUID
	if sid, err := kit.GetConnectRpcSessionUUID(ctx); err == nil {
		callerSessionID = &sid
	}

	keys, revision, err := s.profileService.GetE2EEKeySet(ctx, uuidVal, callerSessionID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(&rpc_personal_profilev1.GetE2EEPublicKeyResponse{
		E2EePublicKeys: keys,
		KeysRevision:   revision,
	}), nil
}
