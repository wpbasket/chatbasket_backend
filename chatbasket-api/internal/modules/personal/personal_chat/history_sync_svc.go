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
	RequestID          uuid.UUID `json:"request_id"`
	RequesterSessionID uuid.UUID `json:"requester_session_id"`
	RequesterPublicKey string    `json:"requester_public_key"`
	ChatsCipher        string    `json:"chats_cipher"`
}

type HistorySyncReadyPayload struct {
	RequestID uuid.UUID `json:"request_id"`
}

// RequestHistorySync handles step ①: secondary requests sync
func (s *chatService) RequestHistorySync(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID, chatsCipher string, usedPrimaryKey string) (uuid.UUID, uuid.UUID, string, error) {
	requestID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to generate request ID")
	}

	expiresAt := time.Now().Add(HistorySyncTTL)

	// Upsert the request (replace on new)
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

	// Find the primary session and public key
	primarySessionID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, userID)
	if err != nil {
		// Log but don't fail, the primary might just be offline or we couldn't find it.
		// ReplayPendingForPrimary will catch it later if they connect.
		return requestID, uuid.Nil, "", nil
	}

	// Validate that the secondary device is using the correct primary key
	primaryPublicKey, err := s.AuthProvider.GetSessionE2EEPublicKey(ctx, primarySessionID)
	if err == nil && primaryPublicKey != nil && *primaryPublicKey != usedPrimaryKey {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusConflict, "key_mismatch", "Primary key mismatch. Please refresh your keys.")
	}

	requesterPublicKey, err := s.AuthProvider.GetSessionE2EEPublicKey(ctx, sessionID)
	if err != nil || requesterPublicKey == nil {
		return uuid.Nil, uuid.Nil, "", kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to get requester public key")
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

// ReplayPendingForPrimary is called when the primary session connects to WS
func (s *chatService) ReplayPendingForPrimary(ctx context.Context, userID uuid.UUID, primarySessionID uuid.UUID) ([]HistorySyncRequestedPayload, error) {
	pending, err := s.PostgresQuerier.GetPendingHistorySyncForUser(ctx, userID)
	if err != nil {
		// Just log and return empty, not a fatal error for connection
		return nil, nil
	}

	var payloads []HistorySyncRequestedPayload
	for _, p := range pending {
		// fetch requester public key
		requesterPublicKey, err := s.AuthProvider.GetSessionE2EEPublicKey(ctx, p.SessionID)
		if err != nil || requesterPublicKey == nil {
			continue // skip if we can't get the public key
		}

		chatsStr := string(p.ChatsJson)
		if len(chatsStr) >= 2 && chatsStr[0] == '"' && chatsStr[len(chatsStr)-1] == '"' {
			chatsStr = chatsStr[1 : len(chatsStr)-1]
		}

		payloads = append(payloads, HistorySyncRequestedPayload{
			RequestID:          p.ID,
			RequesterSessionID: p.SessionID,
			RequesterPublicKey: *requesterPublicKey,
			ChatsCipher:        chatsStr,
		})
	}
	return payloads, nil
}
