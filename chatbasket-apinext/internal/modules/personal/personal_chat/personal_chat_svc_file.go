package personal_chat

import (
	"chatbasket-apinext/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"context"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// File URL Generation
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GenerateMessageFileURLs(ctx context.Context, msg personal_chat_store.Message, viewerID uuid.UUID) (string, string, error) {
	// Ownership guard — matches legacy GenerateMessageFileURLs
	if msg.SenderID != viewerID && msg.RecipientID != viewerID {
		return "", "", kit.NewError(http.StatusForbidden, "forbidden", "not authorized to access this file")
	}

	// Soft-delete guard — treat as no file if user deleted this message
	if (msg.SenderID == viewerID && msg.DeletedBySender) || (msg.RecipientID == viewerID && msg.DeletedByRecipient) {
		return "", "", nil
	}

	if msg.FileID == nil || *msg.FileID == "" {
		return "", "", nil
	}

	bucketID := ChatFilesBucketID

	// Check and refresh tokens if expired
	tokenID := msg.FileTokenID
	tokenSecret := msg.FileTokenSecret
	tokenExpiry := kit.DerefTime(msg.FileTokenExpiry)

	refreshData, refreshed, err := kit.EnsureFreshAvatarTokens(
		msg.FileID, tokenID, tokenSecret, tokenExpiry,
		s.AppwriteStorage.Tokens,
		bucketID,
	)
	if err != nil {
		log.Printf("[GenerateMessageFileURLs] Token refresh failed for message %s: %v", msg.ID, err)
		return "", "", kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to refresh file token")
	}

	if refreshed && refreshData != nil {
		newTokenID := refreshData.TokenID
		newTokenSecret := refreshData.TokenSecret
		tokenID = &newTokenID
		tokenSecret = &newTokenSecret

		// Persist the new tokens
		_ = s.PostgresQueries.UpdateMessageFileToken(ctx, personal_chat_store.UpdateMessageFileTokenParams{
			ID:              msg.ID,
			FileTokenID:     tokenID,
			FileTokenSecret: tokenSecret,
			FileTokenExpiry: &refreshData.TokenExpiry,
		})
	}

	fileData := &kit.AppwriteFileData{
		FileId:     msg.FileID,
		FileToken:  tokenID,
		FileSecret: tokenSecret,
	}

	// viewURL is only generated for media types (image/video/audio) — matches legacy
	viewURL := ""
	isMedia := msg.MessageType == "image" || msg.MessageType == "video" || msg.MessageType == "audio"
	if !isMedia && msg.FileMimeType != nil {
		mime := *msg.FileMimeType
		isMedia = strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
	}
	if isMedia {
		if viewURLPtr := kit.BuildFileViewURL(s.AppwriteStorage.Endpoint, s.AppwriteStorage.Project, bucketID, fileData); viewURLPtr != nil {
			viewURL = *viewURLPtr
		}
	}

	// downloadURL is generated for all file types
	downloadURL := ""
	if downloadURLPtr := kit.BuildFileDownloadURL(s.AppwriteStorage.Endpoint, s.AppwriteStorage.Project, bucketID, fileData); downloadURLPtr != nil {
		downloadURL = *downloadURLPtr
	}

	return viewURL, downloadURL, nil
}

func (s *chatService) GetFileURLHandler(ctx context.Context, payload *GetFileURLPayload, userID uuid.UUID) (*GetFileURLResponse, error) {
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
// File Upload
// ──────────────────────────────────────────────────────────────────────────────

// validateFileType checks that the uploaded file's MIME type matches the declared message_type.
func validateFileType(fh *multipart.FileHeader, messageType string) error {
	mimeType := fh.Header.Get("Content-Type")
	switch messageType {
	case "image":
		if !strings.HasPrefix(mimeType, "image/") {
			return fmt.Errorf("invalid file type for image message")
		}
	case "video":
		if !strings.HasPrefix(mimeType, "video/") {
			return fmt.Errorf("invalid file type for video message")
		}
	case "audio":
		if !strings.HasPrefix(mimeType, "audio/") {
			return fmt.Errorf("invalid file type for audio message")
		}
	case "file":
		// any MIME type allowed for generic file
	default:
		return fmt.Errorf("invalid message type")
	}
	return nil
}

func (s *chatService) UploadFileForMessage(ctx context.Context, params UploadFileForMessageParams) (*personal_chat_store.Message, error) {
	eligibility, err := s.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if err != nil {
		return nil, err
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	// File size validation — matches legacy (100 MB limit)
	if params.FileHeader.Size > MaxFileSize {
		return nil, kit.NewError(http.StatusBadRequest, "file_too_large", "file size exceeds 100MB limit. 100MB allowed.")
	}

	// File type validation — MIME must match declared message_type
	if err := validateFileType(params.FileHeader, params.MessageType); err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_file_type", err.Error())
	}

	chat, chatErr := s.CreateOrGetChat(ctx, params.SenderID, params.RecipientID)
	if chatErr != nil {
		return nil, chatErr
	}

	messageID := uuid.New()
	fileID := "msg_" + messageID.String()
	expiresAt := time.Now().Add(DefaultMessageTTL)

	// Upload file to Appwrite Storage
	uploadResult, err := services.UploadFileFromMultipart(
		s.AppwriteStorage,
		ChatFilesBucketID,
		fileID,
		params.FileHeader,
		services.UploadOptions{
			DeleteExisting: false,
			GenerateTokens: true,
		},
	)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "upload_failed", "Failed to upload file: "+err.Error())
	}

	var tokenID, tokenSecret *string
	if len(uploadResult.TokenIDs) > 0 {
		tokenID = &uploadResult.TokenIDs[0]
	}
	if len(uploadResult.TokenSecrets) > 0 {
		tokenSecret = &uploadResult.TokenSecrets[0]
	}

	content := params.Caption
	if content == "" {
		content = params.FileHeader.Filename
	}

	fileName := params.FileHeader.Filename
	fileSize := params.FileHeader.Size
	fileMimeType := params.FileHeader.Header.Get("Content-Type")

	expiry := time.Now().AddDate(1, 0, 0)
	message, dbErr := s.PostgresQueries.CreateMessageWithFile(ctx, personal_chat_store.CreateMessageWithFileParams{
		ID:                          messageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID,
		RecipientID:                 params.RecipientID,
		Content:                     content,
		MessageType:                 params.MessageType,
		FileID:                      &fileID,
		FileName:                    &fileName,
		FileSize:                    &fileSize,
		FileMimeType:                &fileMimeType,
		FileTokenID:                 tokenID,
		FileTokenSecret:             tokenSecret,
		FileTokenExpiry:             &expiry,
		ExpiresAt:                   expiresAt,
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: new(bool),
	})

	if dbErr != nil {
		// Cleanup uploaded file on DB failure
		go s.DeleteChatFile(context.Background(), fileID)
		return nil, kit.NewError(http.StatusInternalServerError, "message_send_failed", kit.GetPostgresError(dbErr).Message)
	}

	// Update chat status
	_ = s.PostgresQueries.UpdateChatStatus(ctx, personal_chat_store.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: &content,
		LastMessageCreatedAt: message.CreatedAt,
		P1LastMessageType:    &message.MessageType,
		LastMessageSenderID:  message.SenderID,
		LastMessageID:        message.ID,
	})

	return &message, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// File Deletion
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) DeleteChatFile(ctx context.Context, fileID string) {
	_, err := s.AppwriteStorage.Storage.DeleteFile(ChatFilesBucketID, fileID)
	if err != nil {
		log.Printf("[DeleteChatFile] Failed to delete file %s from bucket %s: %v", fileID, ChatFilesBucketID, err)
	}
}
