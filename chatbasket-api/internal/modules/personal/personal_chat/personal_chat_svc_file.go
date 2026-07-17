package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// R2 Presigned URL lifetime (per spec §3.D — 15 minutes)
const r2PresignedURLLifetime = 15 * time.Minute

// Pending upload TTL (per spec §4.A — 2 hours)
const pendingUploadTTL = 2 * time.Hour

// ──────────────────────────────────────────────────────────────────────────────
// File URL Generation (R2 presigned GET — 15 min lifetime)
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GenerateMessageFileURLs(ctx context.Context, msg personal_chat_store.Message, viewerID kit.UserId) (string, string, error) {
	if msg.SenderID != viewerID.UuidUserId && msg.RecipientID != viewerID.UuidUserId {
		return "", "", kit.NewError(http.StatusForbidden, "forbidden", "not authorized to access this file")
	}
	if (msg.SenderID == viewerID.UuidUserId && msg.DeletedBySender) || (msg.RecipientID == viewerID.UuidUserId && msg.DeletedByRecipient) {
		return "", "", nil
	}
	if msg.FileID == nil || *msg.FileID == "" {
		return "", "", nil
	}
	client := s.R2Pool.GetClient(*msg.FileID)
	_, objectKey := clients.ParseFilePrefix(*msg.FileID)
	downloadURL, err := client.GenerateDownloadURL(ctx, client.ChatBucket(), objectKey, r2PresignedURLLifetime)
	if err != nil {
		return "", "", kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate download URL: "+err.Error())
	}
	viewURL := ""
	isMedia := msg.MessageType == "image" || msg.MessageType == "video" || msg.MessageType == "audio"
	if !isMedia && msg.FileMimeType != nil {
		mime := *msg.FileMimeType
		isMedia = strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
	}
	if isMedia {
		viewURL = downloadURL
	}
	return viewURL, downloadURL, nil
}

func (s *chatService) GetFileURLHandler(ctx context.Context, payload *GetFileURLPayload, userID kit.UserId) (*GetFileURLResponse, error) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid message ID")
	}
	message, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Message not found")
	}
	viewURL, downloadURL, fileErr := s.GenerateMessageFileURLs(ctx, message, userID)
	if fileErr != nil {
		return nil, fileErr
	}
	return &GetFileURLResponse{
		ViewURL:     viewURL,
		DownloadURL: downloadURL,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// File Upload — Presign + Confirm (R2 direct upload from client)
// ──────────────────────────────────────────────────────────────────────────────

func validateChatMessageType(messageType string) error {
	switch messageType {
	case "image", "video", "audio", "file":
		return nil
	default:
		return fmt.Errorf("invalid message type for file upload: %s", messageType)
	}
}

// PresignChatUpload picks the next R2 account (round-robin), generates a unique
// file ID with account prefix, registers the upload in pending_uploads with a
// 2-hour TTL, and returns a presigned R2 PUT URL with a 15-minute lifetime.
func (s *chatService) PresignChatUpload(ctx context.Context, params PresignChatUploadParams) (*PresignChatUploadResponse, error) {
	if err := validateChatMessageType(params.MessageType); err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_request", err.Error())
	}
	eligibility, _, _, err := s.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if err != nil {
		return nil, err
	}
	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}
	accountName := s.R2Pool.NextChatAccount()
	client := s.accountClient(accountName)
	objectID := uuid.New().String()
	fileID := clients.BuildFileID(accountName, objectID)
	expiresAt := time.Now().UTC().Add(pendingUploadTTL)
	bucket := client.ChatBucket()
	r2Key := objectID
	if err := s.PendingUploads.Register(ctx, fileID, bucket, r2Key, expiresAt); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "pending_upload_failed", "Failed to register pending upload: "+err.Error())
	}
	presignedURL, err := client.GenerateUploadURL(ctx, bucket, r2Key, r2PresignedURLLifetime)
	if err != nil {
		_ = s.PendingUploads.Remove(ctx, fileID)
		return nil, kit.NewError(http.StatusInternalServerError, "presign_failed", "Failed to generate presigned URL: "+err.Error())
	}
	return &PresignChatUploadResponse{
		FileID:       fileID,
		PresignedURL: presignedURL,
		ExpiresAt:    time.Now().UTC().Add(r2PresignedURLLifetime),
	}, nil
}

// ConfirmChatUpload verifies the pending upload, creates the message record,
// deletes the pending_uploads row, and updates chat status — all in a single
// transaction. Per spec §6.A.3.
func (s *chatService) ConfirmChatUpload(ctx context.Context, params ConfirmChatUploadParams) (*personal_chat_store.Message, error) {
	// Idempotency check:
	if s.PostgresQueries != nil && params.MessageID != uuid.Nil {
		if exists, err := s.PostgresQueries.CheckMessageExists(ctx, params.MessageID); err == nil && exists {
			log.Printf("[E2EE] ConfirmChatUpload: IDEMPOTENT RETRY — message %s already exists. Fetching existing message.", params.MessageID)
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to start confirm transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)

	// 1. Verify pending upload exists (in tx)
	pending, err := s.PendingUploads.LookupTx(ctx, tx, params.FileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusNotFound, "pending_upload_not_found", "No pending upload found. Please restart the upload process.")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "pending_lookup_failed", "Failed to lookup pending upload: "+err.Error())
	}

	// 2. Create or get chat (in tx)
	chat, chatErr := s.createOrGetChatTx(ctx, qtx, params.SenderID.UuidUserId, params.RecipientID)
	if chatErr != nil {
		return nil, chatErr
	}

	// 3. Create the message record (in tx)
	expiresAt := time.Now().Add(DefaultMessageTTL)
	fileID := pending.FileID
	content := params.Content

	message, dbErr := qtx.CreateMessageWithFile(ctx, personal_chat_store.CreateMessageWithFileParams{
		ID:                          params.MessageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID.UuidUserId,
		RecipientID:                 params.RecipientID,
		Content:                     content,
		MessageType:                 params.MessageType,
		FileID:                      &fileID,
		// E2EE media uploads are ciphertext; original name/MIME/size live inside the
		// encrypted message content envelope, not in backend metadata.
		FileName:                    nil,
		FileSize:                    nil,
		FileMimeType:                nil,
		FileTokenID:                 nil,
		FileTokenSecret:             nil,
		FileTokenExpiry:             nil,
		ExpiresAt:                   expiresAt,
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: new(bool),
	})
	if dbErr != nil {
		// Handle PK duplicate key violation (race condition: concurrent confirm requests)
		pgErr := kit.GetPostgresError(dbErr)
		if pgErr.PgError != nil && pgErr.PgError.Code == "23505" {
			log.Printf("[E2EE] ConfirmChatUpload: PK CONFLICT (race) — message %s inserted concurrently. Fetching existing.", params.MessageID)
			tx.Rollback(ctx)
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
		return nil, kit.NewError(http.StatusInternalServerError, "message_create_failed", pgErr.Message)
	}

	// 4. Update chat status (in tx)
	_ = qtx.UpdateChatStatus(ctx, personal_chat_store.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: &content,
		LastMessageCreatedAt: &message.CreatedAt,
		P1LastMessageType:    &message.MessageType,
		LastMessageSenderID:  &message.SenderID,
		LastMessageID:        &message.ID,
	})

	// 5. Delete the pending_uploads row (in tx) — atomic with the insert
	if err := s.PendingUploads.RemoveTx(ctx, tx, params.FileID); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "pending_remove_failed", "Failed to remove pending upload: "+err.Error())
	}

	// 6. Commit — all 4 ops are now atomic
	if err := tx.Commit(ctx); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to commit confirm transaction")
	}
	return &message, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// File Deletion (R2 — idempotent via DeleteFile wrapper)
// ──────────────────────────────────────────────────────────────────────────────

// DeleteChatFile removes a file from R2. Idempotent (handles NoSuchKey internally).
// If account prefix references a missing account (retired per spec §3.E), falls
// back to primary client.
func (s *chatService) DeleteChatFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return nil
	}
	accountName, objectKey := clients.ParseFilePrefix(fileID)
	if accountName == "" {
		return s.R2Pool.PrimaryChatClient().DeleteFile(ctx, s.R2Pool.PrimaryChatClient().ChatBucket(), fileID)
	}
	if !s.R2Pool.HasClient(accountName) {
		log.Printf("[DeleteChatFile] WARNING: Account '%s' not in pool, skipping R2 delete for %s", accountName, fileID)
		return nil
	}
	client := s.R2Pool.GetClient(fileID)
	return client.DeleteFile(ctx, client.ChatBucket(), objectKey)
}

// ──────────────────────────────────────────────────────────────────────────────
// Param Structs
// ──────────────────────────────────────────────────────────────────────────────

type PresignChatUploadParams struct {
	SenderID              kit.UserId
	RecipientID           uuid.UUID
	MessageType           string
	RecipientKeysRevision int32
	SenderKeysRevision    int32
}

type ConfirmChatUploadParams struct {
	MessageID             uuid.UUID
	SenderID              kit.UserId
	RecipientID           uuid.UUID
	FileID                string
	Content               string
	MessageType           string
	IsPrimary             bool
	RecipientKeysRevision int32
	SenderKeysRevision    int32
}
