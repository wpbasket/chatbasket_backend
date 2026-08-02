package personal_contact

import (
	"context"
	"strings"

	"chatbasket-api/internal/platform/kit"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_contactv1 "chatbasket-api/gen/proto/personal/personal_contact"
	rpc_personal_contactv1connect "chatbasket-api/gen/proto/personal/personal_contact/rpc_personal_contactv1connect"

	"connectrpc.com/connect"
)

type contactConnectServer struct {
	contactService *contactService
}

func newContactConnectServer(service *contactService) rpc_personal_contactv1connect.ContactServiceHandler {
	return &contactConnectServer{
		contactService: service,
	}
}

func (s *contactConnectServer) GetContacts(ctx context.Context, req *connect.Request[rpc_personal_contactv1.GetContactsRequest]) (*connect.Response[rpc_personal_contactv1.GetContactsResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res, err := s.contactService.GetContacts(ctx, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

// Stubs for remaining methods to satisfy rpc_personal_contactv1connect.ContactServiceHandler interface
func (s *contactConnectServer) CheckContactExistance(ctx context.Context, req *connect.Request[rpc_personal_contactv1.CheckContactExistanceRequest]) (*connect.Response[rpc_personal_contactv1.CheckContactExistanceResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	if req.Msg.ContactUsername == "" {
		return nil, kit.ParseIntoRpcError(ErrContactUsernameRequired)
	}

	payload := &CheckContactExistancePayload{
		ContactUsername: req.Msg.ContactUsername,
	}

	res, err := s.contactService.CheckContactExistance(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) CreateContact(ctx context.Context, req *connect.Request[rpc_personal_contactv1.CreateContactRequest]) (*connect.Response[rpc_personal_contactv1.CreateContactResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &CreateContactPayload{
		ContactUserId: req.Msg.ContactUserId,
		Nickname:      req.Msg.Nickname,
	}

	if payload.Nickname != nil {
		trimmedNickname := strings.TrimSpace(*payload.Nickname)
		payload.Nickname = &trimmedNickname
	}

	res, err := s.contactService.CreateContact(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) DeleteContact(ctx context.Context, req *connect.Request[rpc_personal_contactv1.DeleteContactRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	if len(req.Msg.ContactUserId) == 0 {
		return nil, kit.ParseIntoRpcError(ErrContactUserIdRequired)
	}

	payload := &DeleteContactPayload{
		ContactUserId: req.Msg.ContactUserId,
	}

	res, err := s.contactService.DeleteContact(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) GetContactRequests(ctx context.Context, req *connect.Request[rpc_personal_contactv1.GetContactRequestsRequest]) (*connect.Response[rpc_personal_contactv1.GetContactRequestsResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	res, err := s.contactService.GetContactRequests(ctx, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) AcceptContactRequest(ctx context.Context, req *connect.Request[rpc_personal_contactv1.AcceptContactRequestRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &AcceptContactRequestPayload{
		ContactUserId: req.Msg.ContactUserId,
	}

	res, err := s.contactService.AcceptContactRequest(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) RejectContactRequest(ctx context.Context, req *connect.Request[rpc_personal_contactv1.RejectContactRequestRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &RejectContactRequestPayload{
		ContactUserId: req.Msg.ContactUserId,
	}

	res, err := s.contactService.RejectContactRequest(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) UndoContactRequest(ctx context.Context, req *connect.Request[rpc_personal_contactv1.UndoContactRequestRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &UndoContactRequestPayload{
		ContactUserId: req.Msg.ContactUserId,
	}

	res, err := s.contactService.UndoContactRequest(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) UpdateContactNickname(ctx context.Context, req *connect.Request[rpc_personal_contactv1.UpdateContactNicknameRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &UpdateContactNicknamePayload{
		ContactUserId: req.Msg.ContactUserId,
		Nickname:      req.Msg.Nickname,
	}

	if payload.Nickname != nil {
		trimmedNickname := strings.TrimSpace(*payload.Nickname)
		payload.Nickname = &trimmedNickname
	}

	res, err := s.contactService.UpdateContactNickname(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) RemoveContactNickname(ctx context.Context, req *connect.Request[rpc_personal_contactv1.RemoveContactNicknameRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &RemoveContactNicknamePayload{
		ContactUserId: req.Msg.ContactUserId,
	}

	res, err := s.contactService.RemoveContactNickname(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) BlockUser(ctx context.Context, req *connect.Request[rpc_personal_contactv1.BlockUserRequest]) (*connect.Response[rpc_personal_contactv1.BlockUserResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &BlockUserPayload{
		BlockedUserId: req.Msg.BlockedUserId,
	}

	res, err := s.contactService.BlockUser(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) UnblockUser(ctx context.Context, req *connect.Request[rpc_personal_contactv1.UnblockUserRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(ErrInvalidPayload)
	}

	payload := &UnblockUserPayload{
		BlockedUserId: req.Msg.BlockedUserId,
	}

	res, err := s.contactService.UnblockUser(ctx, payload, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *contactConnectServer) GetBlocks(ctx context.Context, req *connect.Request[rpc_personal_contactv1.GetBlocksRequest]) (*connect.Response[rpc_personal_contactv1.GetBlocksResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res, err := s.contactService.GetBlocks(ctx, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}
