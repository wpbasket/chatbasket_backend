package personal_chat

import (
	"net/http"

	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HistorySyncRequestPayload struct {
	ChatsCipher    string `json:"chats_cipher"`
	UsedPrimaryKey string `json:"used_primary_key"`
}

type HistorySyncRequestResponse struct {
	RequestID uuid.UUID `json:"request_id"`
}

type HistorySyncUploadPayload struct {
	RequestID     uuid.UUID `json:"request_id"`
	PayloadCipher string    `json:"payload_cipher"`
}

type HistorySyncResponse struct {
	PayloadCipher string `json:"payload_cipher"`
}

type HistorySyncAckPayload struct {
	RequestID uuid.UUID `json:"request_id"`
}

type HistorySyncFetchResponse struct {
	ChatsCipher        string `json:"chats_cipher"`
	RequesterPublicKey string `json:"requester_public_key"`
}

// RequestHistorySync handles POST /chat/history-sync/request
func (h *chatHandler) RequestHistorySync(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	sessionUUIDVal, ok := c.Get("sessionUUID").(uuid.UUID)
	if !ok {
		return kit.NewError(http.StatusUnauthorized, "unauthorized", "invalid session")
	}

	var req HistorySyncRequestPayload
	if err := c.Bind(&req); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid payload")
	}

	if req.UsedPrimaryKey == "" {
		return kit.NewError(http.StatusBadRequest, "bad_request", "used_primary_key is required")
	}

	requestID, primarySessionID, requesterPubKey, err := h.Service.RequestHistorySync(
		c.Request().Context(),
		userID.UuidUserId,
		sessionUUIDVal,
		req.ChatsCipher,
		req.UsedPrimaryKey,
	)
	if err != nil {
		return err
	}

	// SSE Broadcast: RequestHistorySyncSseEvent to primary device.
	// Id-only pointer: the cipher must NOT ride the event — pg_notify caps
	// payloads at 8,000 bytes and a full have_ids cipher exceeds it. The
	// primary pulls the body via FetchHistorySync (same pattern as the
	// Upload → Download reverse leg).
	if h.personalSseManager != nil && primarySessionID != uuid.Nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_RequestHistorySyncSseEvent{
						RequestHistorySyncSseEvent: &rpc_personal_chatv1.RequestHistorySyncSsePayload{
							RequestId:          requestID.String(),
							RequesterPublicKey: requesterPubKey,
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUserSession(userID.UuidUserId, primarySessionID, sseEvent)
	}

	return c.JSON(http.StatusOK, HistorySyncRequestResponse{
		RequestID: requestID,
	})
}

// UploadHistorySync handles POST /chat/history-sync/upload
func (h *chatHandler) UploadHistorySync(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	if !extractIsPrimary(c) {
		return kit.NewError(http.StatusForbidden, "forbidden", "Only primary device can upload history sync")
	}

	var req HistorySyncUploadPayload
	if err := c.Bind(&req); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid payload")
	}

	requesterSessionID, err := h.Service.UploadHistorySync(c.Request().Context(), userID.UuidUserId, req.RequestID, req.PayloadCipher)
	if err != nil {
		return err
	}

	// SSE Broadcast: UploadHistorySyncSseEvent to requesting secondary device
	if h.personalSseManager != nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_UploadHistorySyncSseEvent{
						UploadHistorySyncSseEvent: &rpc_personal_chatv1.UploadHistorySyncSsePayload{
							RequestId: req.RequestID.String(),
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUserSession(userID.UuidUserId, requesterSessionID, sseEvent)
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true})
}

// DownloadHistorySync handles GET /chat/history-sync?request_id=...
func (h *chatHandler) DownloadHistorySync(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	sessionUUIDVal, ok := c.Get("sessionUUID").(uuid.UUID)
	if !ok {
		return kit.NewError(http.StatusUnauthorized, "unauthorized", "invalid session")
	}

	requestIDStr := c.QueryParam("request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "missing or invalid request_id")
	}

	payloadCipher, err := h.Service.DownloadHistorySync(c.Request().Context(), userID.UuidUserId, sessionUUIDVal, requestID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, HistorySyncResponse{
		PayloadCipher: *payloadCipher,
	})
}

// FetchHistorySync handles GET /chat/history-sync/fetch?request_id=...
// Primary-only pull of a secondary's request body (pointer pattern).
func (h *chatHandler) FetchHistorySync(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	if !extractIsPrimary(c) {
		return kit.NewError(http.StatusForbidden, "forbidden", "Only primary device can fetch history sync requests")
	}

	requestIDStr := c.QueryParam("request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "missing or invalid request_id")
	}

	chatsCipher, requesterPubKey, err := h.Service.FetchHistorySync(c.Request().Context(), userID.UuidUserId, requestID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, HistorySyncFetchResponse{
		ChatsCipher:        chatsCipher,
		RequesterPublicKey: requesterPubKey,
	})
}

// AcknowledgeHistorySync handles POST /chat/history-sync/ack
func (h *chatHandler) AcknowledgeHistorySync(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	sessionUUIDVal, ok := c.Get("sessionUUID").(uuid.UUID)
	if !ok {
		return kit.NewError(http.StatusUnauthorized, "unauthorized", "invalid session")
	}

	var req HistorySyncAckPayload
	if err := c.Bind(&req); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid payload")
	}

	if req.RequestID == uuid.Nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "request_id is required")
	}

	if err := h.Service.AcknowledgeHistorySync(c.Request().Context(), userID.UuidUserId, sessionUUIDVal, req.RequestID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true})
}
