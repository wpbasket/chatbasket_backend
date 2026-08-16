package core_auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_core_authv1 "chatbasket-api/gen/proto/core/core_auth"
	rpc_core_authv1connect "chatbasket-api/gen/proto/core/core_auth/rpc_core_authv1connect"

	"connectrpc.com/connect"
)

type authConnectServer struct {
	authService        *AuthService
	personalSseManager *personal_sse.Manager
	qrHub              *QRHub
}

func newAuthConnectServer(authService *AuthService, personalSseManager *personal_sse.Manager, qrHub *QRHub) rpc_core_authv1connect.AuthServiceHandler {
	return &authConnectServer{
		authService:        authService,
		personalSseManager: personalSseManager,
		qrHub:              qrHub,
	}
}

func (s *authConnectServer) setWebCookies(header http.Header, origin string, user *rpc_core_authv1.SessionResponse) error {
	expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
	if err != nil {
		return err
	}

	isLocal := strings.Contains(origin, "localhost:8081")
	cookieDomain := "chatbasket.live"
	cookieSecure := true
	if isLocal {
		cookieDomain = ""
		cookieSecure = false
	}

	sessionCookie := &http.Cookie{
		Name:     "sessionId",
		Value:    user.SessionId,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiry,
	}

	userCookie := &http.Cookie{
		Name:     "userId",
		Value:    user.UserId,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiry,
	}

	header.Add("Set-Cookie", sessionCookie.String())
	header.Add("Set-Cookie", userCookie.String())
	return nil
}

func (s *authConnectServer) clearWebCookies(header http.Header, origin string) {
	isLocal := strings.Contains(origin, "localhost:8081")
	cookieDomain := "chatbasket.live"
	cookieSecure := true
	if isLocal {
		cookieDomain = ""
		cookieSecure = false
	}

	sessionCookie := &http.Cookie{
		Name:     "sessionId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		MaxAge:   -1,
	}
	userCookie := &http.Cookie{
		Name:     "userId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		MaxAge:   -1,
	}

	header.Add("Set-Cookie", sessionCookie.String())
	header.Add("Set-Cookie", userCookie.String())
}



// AuthService implementations

func (s *authConnectServer) Signup(ctx context.Context, req *connect.Request[rpc_core_authv1.SignupRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.Email == "" || req.Msg.Password == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &SignupPayload{
		Name:     req.Msg.Name,
		Email:    req.Msg.Email,
		Password: req.Msg.Password,
	}

	res, err := s.authService.Signup(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) AccountVerification(ctx context.Context, req *connect.Request[rpc_core_authv1.AccountVerificationRequest]) (*connect.Response[rpc_core_authv1.SessionResponse], error) {
	if req.Msg.Email == "" || req.Msg.Secret == "" || req.Msg.Platform == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &AuthVerificationPayload{
		Email:    req.Msg.Email,
		Secret:   req.Msg.Secret,
		Platform: req.Msg.Platform,
	}

	user, err := s.authService.AccountVerification(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res := connect.NewResponse(user)

	if req.Msg.Platform == "web" {
		origin := req.Header().Get("Origin")
		if err := s.setWebCookies(res.Header(), origin, user); err != nil {
			return nil, kit.ParseIntoRpcError(ErrInvalidExpiryFormat)
		}
		// Redact sensitive session fields for web
		res.Msg.SessionId = ""
	}

	return res, nil
}

func (s *authConnectServer) Login(ctx context.Context, req *connect.Request[rpc_core_authv1.LoginRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.Email == "" || req.Msg.Password == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &LoginPayload{
		Email:    req.Msg.Email,
		Password: req.Msg.Password,
	}

	res, err := s.authService.Login(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) LoginVerification(ctx context.Context, req *connect.Request[rpc_core_authv1.LoginVerificationRequest]) (*connect.Response[rpc_core_authv1.SessionResponse], error) {
	if req.Msg.Email == "" || req.Msg.Secret == "" || req.Msg.Platform == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &AuthVerificationPayload{
		Email:    req.Msg.Email,
		Secret:   req.Msg.Secret,
		Platform: req.Msg.Platform,
	}

	user, err := s.authService.LoginVerification(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res := connect.NewResponse(user)

	if req.Msg.Platform == "web" {
		origin := req.Header().Get("Origin")
		if err := s.setWebCookies(res.Header(), origin, user); err != nil {
			return nil, kit.ParseIntoRpcError(ErrInvalidExpiryFormat)
		}
		// Redact sensitive session fields for web
		res.Msg.SessionId = ""
	}

	return res, nil
}

func (s *authConnectServer) ResendOTP(ctx context.Context, req *connect.Request[rpc_core_authv1.ResendOTPRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.Email == "" || req.Msg.Type == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &ResendOTPPayload{
		Email: req.Msg.Email,
		Type:  req.Msg.Type,
	}

	res, err := s.authService.ResendOTP(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) ForgotPassword(ctx context.Context, req *connect.Request[rpc_core_authv1.ForgotPasswordRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.Email == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &ForgotPasswordPayload{
		Email: req.Msg.Email,
	}

	res, err := s.authService.ForgotPassword(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) VerifyForgotPassword(ctx context.Context, req *connect.Request[rpc_core_authv1.VerifyForgotPasswordRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	if req.Msg.UpdateId == "" || req.Msg.Otp == "" || req.Msg.NewPassword == "" {
		return nil, kit.ParseIntoRpcError(ErrMissingRequired)
	}

	payload := &ForgotPasswordVerifyPayload{
		UpdateID:    req.Msg.UpdateId,
		Otp:         req.Msg.Otp,
		NewPassword: req.Msg.NewPassword,
	}

	res, err := s.authService.VerifyForgotPassword(ctx, payload)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) Logout(ctx context.Context, req *connect.Request[rpc_core_authv1.LogoutRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}
	sessionId, err := kit.GetConnectRpcSessionID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidSessionContext)
	}
	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}
	platform, err := kit.GetConnectRpcPlatform(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	payload := &LogoutPayload{
		AllSessions: req.Msg.AllSessions,
	}

	if isPrimary {
		payload.AllSessions = true
	}

	res, err := s.authService.Logout(ctx, payload, userID.UuidUserId, sessionId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if s.personalSseManager != nil {
		if payload.AllSessions {
			s.personalSseManager.UnregisterUserConnections(userID.UuidUserId)
		} else {
			// Only the single-session branch needs the session row id. Looked up here (and
			// skipped on failure) so a missing value can never abort an otherwise successful
			// logout — identical to the REST twin in core_auth_api_common_http_handler.go.
			if sessionUUID, uuidErr := kit.GetConnectRpcSessionUUID(ctx); uuidErr == nil {
				s.personalSseManager.UnregisterSession(userID.UuidUserId, sessionUUID)
			}
		}
	}

	resp := connect.NewResponse(res)

	if platform == "web" {
		origin := req.Header().Get("Origin")
		s.clearWebCookies(resp.Header(), origin)
	}

	return resp, nil
}

func (s *authConnectServer) GetUser(ctx context.Context, req *connect.Request[rpc_core_authv1.GetUserRequest]) (*connect.Response[rpc_core_authv1.SessionResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.NewConnectRpcError(401, "unauthorized", "Invalid user context")
	}
	sessionId, err := kit.GetConnectRpcSessionID(ctx)
	if err != nil {
		return nil, kit.NewConnectRpcError(401, "unauthorized", "No session context")
	}
	platform, err := kit.GetConnectRpcPlatform(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	user, err := s.authService.GetUserWithSession(ctx, userID.UuidUserId, sessionId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res := connect.NewResponse(user)
	if platform == "web" {
		res.Msg.SessionId = ""
	}

	return res, nil
}

func (s *authConnectServer) RequestUpdateOTP(ctx context.Context, req *connect.Request[rpc_core_authv1.RequestUpdateOTPRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	payload := &RequestUpdateOTPPayload{
		UpdateType: req.Msg.UpdateType,
	}

	res, err := s.authService.RequestUpdateOTP(ctx, payload, userID.UuidUserId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) ConfirmPasswordUpdate(ctx context.Context, req *connect.Request[rpc_core_authv1.ConfirmPasswordUpdateRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	payload := &ConfirmPasswordUpdatePayload{
		UpdateID:    req.Msg.UpdateId,
		Otp:         req.Msg.Otp,
		NewPassword: req.Msg.NewPassword,
	}

	res, err := s.authService.ConfirmPasswordUpdate(ctx, payload, userID.UuidUserId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) RequestEmailUpdate(ctx context.Context, req *connect.Request[rpc_core_authv1.RequestEmailUpdateRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	payload := &RequestEmailUpdatePayload{
		NewEmail: req.Msg.NewEmail,
		Password: req.Msg.Password,
	}

	res, err := s.authService.RequestEmailUpdate(ctx, payload, userID.UuidUserId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) ConfirmEmailUpdate(ctx context.Context, req *connect.Request[rpc_core_authv1.ConfirmEmailUpdateRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidUserContext)
	}

	payload := &ConfirmEmailUpdatePayload{
		UpdateID: req.Msg.UpdateId,
		Otp:      req.Msg.Otp,
	}

	res, err := s.authService.ConfirmEmailUpdate(ctx, payload, userID.UuidUserId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) QRInitiate(ctx context.Context, req *connect.Request[rpc_core_authv1.QRInitiateRequest]) (*connect.Response[rpc_core_authv1.QRInitiateResponse], error) {
	res, err := s.authService.QRInitiate(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) QRApprove(ctx context.Context, req *connect.Request[rpc_core_authv1.QRApproveRequest]) (*connect.Response[rpc_core_authv1.QRApproveResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.NewConnectRpcError(http.StatusUnauthorized, "unauthorized", "User ID not found in context")
	}

	res, err := s.authService.QRApprove(ctx, userID.UuidUserId, req.Msg.QrToken)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *authConnectServer) QRCallback(ctx context.Context, req *connect.Request[rpc_core_authv1.QRCallbackRequest]) (*connect.Response[rpc_core_authv1.SessionResponse], error) {
	token, err := s.authService.ParseAndVerifyQRToken(req.Msg.QrToken)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	user, err := s.authService.QRCallback(ctx, req.Msg.QrToken, "web")
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// Close the waiting QR WebSocket
	if s.qrHub != nil {
		s.qrHub.Close(token)
	}

	res := connect.NewResponse(user)

	origin := req.Header().Get("Origin")
	if err := s.setWebCookies(res.Header(), origin, user); err != nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidExpiryFormat)
	}
	res.Msg.SessionId = ""

	return res, nil
}
