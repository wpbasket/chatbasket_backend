package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
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
		func(uid uuid.UUID, sid uuid.UUID) bool {
			return h.hub.IsSessionActive(uid, sid)
		},
	)
	if err != nil {
		return err
	}

	// Send WS event to primary device
	if primarySessionID != uuid.Nil {
		wsPayload := HistorySyncRequestedPayload{
			RequestID:          requestID,
			RequesterSessionID: sessionUUIDVal,
			RequesterPublicKey: requesterPubKey,
			ChatsCipher:        req.ChatsCipher,
		}
		go h.hub.BroadcastToUserSession(userID.UuidUserId, primarySessionID.String(), websocket.WSEvent{
			Type:    WSEventHistorySyncRequested,
			Payload: wsPayload,
		})
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

	// Notify the requesting secondary device that it's ready
	wsPayload := HistorySyncReadyPayload{
		RequestID: req.RequestID,
	}
	go h.hub.BroadcastToUserSession(userID.UuidUserId, requesterSessionID.String(), websocket.WSEvent{
		Type:    WSEventHistorySyncReady,
		Payload: wsPayload,
	})

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
