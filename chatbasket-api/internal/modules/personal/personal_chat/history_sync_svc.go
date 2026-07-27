package personal_chat

import (
	"context"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type HistorySyncRequestedPayload struct {
	RequestID          uuid.UUID `json:"requestId"`
	RequesterSessionID uuid.UUID `json:"requesterSessionId"`
	RequesterPublicKey string    `json:"requesterPublicKey"`
	ChatsCipher        string    `json:"chatsCipher"`
}

type HistorySyncReadyPayload struct {
	RequestID uuid.UUID `json:"requestId"`
}

// RequestHistorySync handles step ①: secondary requests sync
func (s *chatService) RequestHistorySync(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID, chatsCipher string, usedPrimaryKey string, isPrimaryActive func(uuid.UUID, uuid.UUID) bool) (uuid.UUID, uuid.UUID, string, error) {
	// 1. Find the primary session
	primarySessionID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, userID)
	if err != nil || primarySessionID == uuid.Nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusServiceUnavailable, "primary_offline", "Primary device is not connected")
	}

	// 2. Check if primary is actively connected to the WebSocket
	if isPrimaryActive != nil && !isPrimaryActive(userID, primarySessionID) {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusServiceUnavailable, "primary_offline", "Primary device is not connected")
	}

	// 3. Validate that the secondary device is using the correct primary key
	primaryPublicKey, err := s.AuthProvider.GetSessionE2EEPublicKey(ctx, primarySessionID)
	if err == nil && primaryPublicKey != nil && *primaryPublicKey != usedPrimaryKey {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusConflict, "key_mismatch", "Primary key mismatch. Please refresh your keys.")
	}

	requesterPublicKey, err := s.AuthProvider.GetSessionE2EEPublicKey(ctx, sessionID)
	if err != nil || requesterPublicKey == nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to get requester public key")
	}

	requestID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to generate request ID")
	}

	expiresAt := time.Now().Add(HistorySyncTTL)

	// 4. Upsert the request (replace on new) ONLY if primary is active
	_, err = s.PostgresQuerier.UpsertHistorySync(ctx, personal_chat_store.UpsertHistorySyncParams{
		ID:        requestID,
		UserID:    userID,
		SessionID: sessionID,
		ChatsJson: []byte(chatsCipher),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to upsert history sync request: "+err.Error())
	}

	return requestID, primarySessionID, *requesterPublicKey, nil
}

// UploadHistorySync handles step ③: primary uploads E2EE payload for secondary
func (s *chatService) UploadHistorySync(ctx context.Context, userID uuid.UUID, requestID uuid.UUID, payloadCipher string) (uuid.UUID, error) {
	// We need to fetch the session ID associated with this requestID to send the "ready" event.
	syncReq, err := s.PostgresQuerier.GetHistorySyncMeta(ctx, requestID)
	if err != nil {
		return uuid.Nil, kit.NewError(http.StatusGone, "request_gone", "History sync request expired or not found")
	}

	// Ensure it belongs to this user
	if syncReq.UserID != userID {
		return uuid.Nil, kit.NewError(http.StatusForbidden, "forbidden", "Not your request")
	}

	if syncReq.ExpiresAt.Before(time.Now()) {
		return uuid.Nil, kit.NewError(http.StatusGone, "request_gone", "History sync request expired")
	}

	// Update payload
	payloadBytes := []byte(payloadCipher)
	rowsAffected, err := s.PostgresQuerier.UploadHistorySyncPayload(ctx, personal_chat_store.UploadHistorySyncPayloadParams{
		Payload: payloadBytes, // store as JSON string
		ID:      requestID,
		UserID:  userID,
	})
	if err != nil {
		return uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to update payload: "+err.Error())
	}
	if rowsAffected == 0 {
		return uuid.Nil, kit.NewError(http.StatusGone, "request_gone", "History sync request expired or not found")
	}

	return syncReq.SessionID, nil
}

// DownloadHistorySync handles step ⑤: secondary downloads payload
func (s *chatService) DownloadHistorySync(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID, requestID uuid.UUID) (*string, error) {
	syncReqMeta, err := s.PostgresQuerier.GetHistorySyncMeta(ctx, requestID)
	if err != nil {
		return nil, kit.NewError(http.StatusGone, "request_gone", "History sync request expired or not found")
	}

	if syncReqMeta.UserID != userID || syncReqMeta.SessionID != sessionID {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "Not your request")
	}

	if syncReqMeta.ExpiresAt.Before(time.Now()) {
		return nil, kit.NewError(http.StatusGone, "request_gone", "History sync request expired")
	}

	payload, err := s.PostgresQuerier.GetHistorySyncForDownload(ctx, personal_chat_store.GetHistorySyncForDownloadParams{
		ID:        requestID,
		SessionID: sessionID,
	})
	if err != nil || len(payload) == 0 {
		return nil, kit.NewError(http.StatusNotFound, "not_ready", "Payload not yet ready")
	}

	// Clean quotes from the JSON string
	payloadStr := string(payload)
	if len(payloadStr) >= 2 && payloadStr[0] == '"' && payloadStr[len(payloadStr)-1] == '"' {
		payloadStr = payloadStr[1 : len(payloadStr)-1]
	}

	return &payloadStr, nil
}


