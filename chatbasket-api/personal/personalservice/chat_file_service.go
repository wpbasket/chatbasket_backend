package personalservice

import (
	"chatbasket-api/internal/db/personal"
	"chatbasket-api/model"
	"chatbasket-api/services"
	"chatbasket-api/utils"
	"context"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	MaxFileSize       = 100 * 1024 * 1024
	ChatFilesBucketID = "6995c8f4002e4d744b3b"
)

type UploadFileForMessageParams struct {
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	FileHeader  *multipart.FileHeader
	MessageType string
	Caption     string
	IsPrimary   bool
}

func (ps *Service) UploadFileForMessage(ctx context.Context, params UploadFileForMessageParams) (*personal.Message, *model.ApiError) {
	log.Printf("[ChatFileService] UploadFileForMessage starting. Sender: %s, Recipient: %s, FileName: %s, Size: %d",
		params.SenderID, params.RecipientID, params.FileHeader.Filename, params.FileHeader.Size)

	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		log.Printf("[ChatFileService] Eligibility check failed: %v", apiErr)
		return nil, apiErr
	}
	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	if params.FileHeader.Size > MaxFileSize {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "file size exceeds 100MB limit. 100MB allowed.",
			Type:    "file_too_large",
		}
	}

	if err := validateFileType(params.FileHeader, params.MessageType); err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
			Type:    "invalid_file_type",
		}
	}

	chat, apiErr := ps.CreateOrGetChat(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		log.Printf("[ChatFileService] CreateOrGetChat failed: %v", apiErr)
		return nil, apiErr
	}

	fileID := uuid.New().String()
	log.Printf("[ChatFileService] Starting Appwrite upload. FileID: %s", fileID)

	uploadResult, apiErr := ps.UploadFileFromMultipart(
		ChatFilesBucketID,
		fileID,
		params.FileHeader,
		services.UploadOptions{
			DeleteExisting: false,
			GenerateTokens: true,
		},
	)
	if apiErr != nil {
		log.Printf("[ChatFileService] Appwrite upload failed: %v", apiErr)
		return nil, apiErr
	}
	log.Printf("[ChatFileService] Appwrite upload success. Tokens generated.")

	if len(uploadResult.TokenIDs) == 0 || len(uploadResult.TokenSecrets) == 0 {
		ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, fileID)
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "file token generation failed",
			Type:    "internal_server_error",
		}
	}

	tokenExpiry, err := time.Parse(time.RFC3339, uploadResult.Expire)
	if err != nil {
		ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, fileID)
		ps.Appwrite.Tokens.Delete(uploadResult.TokenIDs[0])
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to parse token expiry",
			Type:    "internal_server_error",
		}
	}

	messageID := uuid.New()
	expiresAt := time.Now().Add(DefaultMessageTTL)

	log.Printf("[ChatFileService] Creating DB record for message...")
	message, err := ps.PersonalQueries.CreateMessageWithFile(ctx, personal.CreateMessageWithFileParams{
		ID:                          messageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID,
		RecipientID:                 params.RecipientID,
		Content:                     params.Caption,
		MessageType:                 params.MessageType,
		FileID:                      &fileID,
		FileName:                    &params.FileHeader.Filename,
		FileSize:                    &params.FileHeader.Size,
		FileMimeType:                new(params.FileHeader.Header.Get("Content-Type")),
		FileTokenID:                 &uploadResult.TokenIDs[0],
		FileTokenSecret:             &uploadResult.TokenSecrets[0],
		FileTokenExpiry:             pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
		ExpiresAt:                   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: new(bool),
	})

	if err != nil {
		log.Printf("[ChatFileService] DB create failed: %v. Deleting Appwrite file.", err)
		ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, fileID)
		ps.Appwrite.Tokens.Delete(uploadResult.TokenIDs[0])

		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "message_send_failed",
		}
	}

	log.Printf("[ChatFileService] Upload complete. MessageID: %s", message.ID)

	// Update chat status (Last Message + Unread Count)
	// We ignore error here to not fail the request if the message was sent successfully
	// but we should log it.

	// Determine fallback preview if caption is empty
	previewContent := params.Caption
	if previewContent == "" {
		previewContent = params.FileHeader.Filename
	}

	msgType := message.MessageType
	senderID := message.SenderID

	_ = ps.PersonalQueries.UpdateChatStatus(ctx, personal.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: &previewContent,
		LastMessageCreatedAt: message.CreatedAt,
		P1LastMessageType:    &msgType,
		LastMessageSenderID:  pgtype.UUID{Bytes: senderID, Valid: true},
		LastMessageID:        pgtype.UUID{Bytes: message.ID, Valid: true},
	})

	return &message, nil
}

func (ps *Service) GetMessageFileURL(ctx context.Context, messageID uuid.UUID, userID uuid.UUID) (string, *model.ApiError) {
	message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return "", &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "message not found",
			Type:    "not_found",
		}
	}

	if message.SenderID != userID && message.RecipientID != userID {
		return "", &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "not authorized to access this file",
			Type:    "forbidden",
		}
	}

	if (message.SenderID == userID && message.DeletedBySender) || (message.RecipientID == userID && message.DeletedByRecipient) {
		return "", &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "message file not found",
			Type:    "not_found",
		}
	}

	if message.FileID == nil || *message.FileID == "" {
		return "", &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "no file attached to this message",
			Type:    "not_found",
		}
	}

	now := time.Now().UTC()
	needsRefresh := false

	if message.FileTokenExpiry.Valid {
		needsRefresh = !message.FileTokenExpiry.Time.UTC().After(now)
	} else {
		needsRefresh = true
	}

	if message.FileTokenID == nil || *message.FileTokenID == "" ||
		message.FileTokenSecret == nil || *message.FileTokenSecret == "" {
		needsRefresh = true
	}

	var tokenID, tokenSecret string

	if needsRefresh {
		// PROPER FLOW: 1. Delete existing tokens first
		if message.FileTokenID != nil && *message.FileTokenID != "" {
			if _, err := ps.Appwrite.Tokens.Delete(*message.FileTokenID); err != nil {
				log.Printf("[Appwrite] failed to delete existing token %s: %v", *message.FileTokenID, err)
			}
		}

		// 2. Create new token
		exp := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
		newToken, err := ps.Appwrite.Tokens.CreateFileToken(
			ChatFilesBucketID,
			*message.FileID,
			ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp),
		)
		if err != nil {
			return "", &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "failed to refresh file token",
				Type:    "internal_server_error",
			}
		}

		tokenID = newToken.Id
		tokenSecret = newToken.Secret

		// 3. Update database with new token
		tokenExpiry, _ := time.Parse(time.RFC3339, newToken.Expire)
		err = ps.PersonalQueries.UpdateMessageFileToken(ctx, personal.UpdateMessageFileTokenParams{
			ID:              messageID,
			FileTokenID:     &tokenID,
			FileTokenSecret: &tokenSecret,
			FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
		})
		if err != nil {
			// If database update fails, clean up the new token
			if _, deleteErr := ps.Appwrite.Tokens.Delete(tokenID); deleteErr != nil {
				log.Printf("[Appwrite] failed to cleanup new token %s after DB error: %v", tokenID, deleteErr)
			}
		}
	} else {
		// Add safety check before dereferencing tokens
		if message.FileTokenID != nil && message.FileTokenSecret != nil &&
			*message.FileTokenID != "" && *message.FileTokenSecret != "" {
			tokenID = *message.FileTokenID
			tokenSecret = *message.FileTokenSecret
		} else {
			// Fallback to refresh if tokens are invalid
			// PROPER FLOW: 1. Delete any existing tokens first
			if message.FileTokenID != nil && *message.FileTokenID != "" {
				if _, err := ps.Appwrite.Tokens.Delete(*message.FileTokenID); err != nil {
					log.Printf("[Appwrite] failed to delete existing token %s: %v", *message.FileTokenID, err)
				}
			}

			// 2. Create new token
			exp := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
			newToken, err := ps.Appwrite.Tokens.CreateFileToken(
				ChatFilesBucketID,
				*message.FileID,
				ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp),
			)
			if err != nil {
				return "", &model.ApiError{
					Code:    http.StatusInternalServerError,
					Message: "failed to refresh file token",
					Type:    "internal_server_error",
				}
			}

			tokenID = newToken.Id
			tokenSecret = newToken.Secret

			// 3. Update database with new token
			tokenExpiry, _ := time.Parse(time.RFC3339, newToken.Expire)
			err = ps.PersonalQueries.UpdateMessageFileToken(ctx, personal.UpdateMessageFileTokenParams{
				ID:              messageID,
				FileTokenID:     &tokenID,
				FileTokenSecret: &tokenSecret,
				FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
			})
			if err != nil {
				// If database update fails, clean up the new token
				if _, deleteErr := ps.Appwrite.Tokens.Delete(tokenID); deleteErr != nil {
					log.Printf("[Appwrite] failed to cleanup new token %s after DB error: %v", tokenID, deleteErr)
				}
			}
		}
	}

	fileURL := utils.BuildFileViewURL(
		ps.Appwrite.Endpoint,
		ps.Appwrite.ProjectID,
		ChatFilesBucketID,
		&utils.AppwriteFileData{
			FileId:     message.FileID,
			FileToken:  &tokenID,
			FileSecret: &tokenSecret,
		},
	)

	if fileURL != nil {
		log.Printf("[ChatFile] Resolved URL: %s", *fileURL)
	}

	if fileURL == nil {
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to build file URL",
			Type:    "internal_server_error",
		}
	}

	return *fileURL, nil
}

func (ps *Service) CleanupMessageFile(ctx context.Context, messageID uuid.UUID) error {
	message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return err
	}

	if message.FileID != nil && *message.FileID != "" {
		ps.deleteAllFileTokens(ChatFilesBucketID, *message.FileID)
		if _, err := ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, *message.FileID); err != nil {
			log.Printf("[Appwrite] failed to delete file %s: %v", *message.FileID, err)
		}
	}

	if message.ThumbnailFileID != nil && *message.ThumbnailFileID != "" {
		ps.deleteAllFileTokens(ChatFilesBucketID, *message.ThumbnailFileID)
		if _, err := ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, *message.ThumbnailFileID); err != nil {
			log.Printf("[Appwrite] failed to delete thumbnail %s: %v", *message.ThumbnailFileID, err)
		}
	}

	return nil
}

// DeleteChatFile handles the cleanup of a single file and its tokens.
func (ps *Service) DeleteChatFile(ctx context.Context, fileID string) {
	if fileID == "" {
		return
	}
	ps.deleteAllFileTokens(ChatFilesBucketID, fileID)
	if _, err := ps.Appwrite.Storage.DeleteFile(ChatFilesBucketID, fileID); err != nil {
		log.Printf("[Appwrite] failed to delete file %s: %v", fileID, err)
	}
}

func (ps *Service) deleteAllFileTokens(bucketID, fileID string) {
	if fileID == "" {
		return
	}

	tokenList, err := ps.Appwrite.Tokens.List(bucketID, fileID)
	if err != nil {
		log.Printf("[Appwrite] failed to list tokens for file %s: %v", fileID, err)
		return
	}

	for _, token := range tokenList.Tokens {
		if _, err := ps.Appwrite.Tokens.Delete(token.Id); err != nil {
			log.Printf("[Appwrite] failed to delete token %s for file %s: %v", token.Id, fileID, err)
		}
	}
}

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
	default:
		return fmt.Errorf("invalid message type")
	}

	return nil
}

// GenerateMessageFileURL is a helper that generates a signed URL for a message's file,
// GenerateMessageFileURLs is a helper that generates signed View and Download URLs for a message's file,
// handling token refresh internally if needed. This is optimized for bulk message processing.
func (ps *Service) GenerateMessageFileURLs(ctx context.Context, message personal.Message, userID uuid.UUID) (string, string, *model.ApiError) {
	if message.SenderID != userID && message.RecipientID != userID {
		return "", "", &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "not authorized to access this file",
			Type:    "forbidden",
		}
	}

	if (message.SenderID == userID && message.DeletedBySender) || (message.RecipientID == userID && message.DeletedByRecipient) {
		return "", "", nil // Treat as "no file" if the user deleted the message
	}

	if message.FileID == nil || *message.FileID == "" {
		return "", "", nil // No file, no URLs
	}

	now := time.Now().UTC()
	needsRefresh := false

	if message.FileTokenExpiry.Valid {
		needsRefresh = !message.FileTokenExpiry.Time.UTC().After(now)
	} else {
		needsRefresh = true
	}

	if message.FileTokenID == nil || *message.FileTokenID == "" ||
		message.FileTokenSecret == nil || *message.FileTokenSecret == "" {
		needsRefresh = true
	}

	var tokenID, tokenSecret string

	if needsRefresh {
		// PROPER FLOW: 1. Delete existing tokens first
		if message.FileTokenID != nil && *message.FileTokenID != "" {
			if _, err := ps.Appwrite.Tokens.Delete(*message.FileTokenID); err != nil {
				log.Printf("[Appwrite] failed to delete existing token %s: %v", *message.FileTokenID, err)
			}
		}

		// 2. Create new token
		exp := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
		newToken, err := ps.Appwrite.Tokens.CreateFileToken(
			ChatFilesBucketID,
			*message.FileID,
			ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp),
		)
		if err != nil {
			log.Printf("[GenerateMessageFileURLs] token refresh failed for file %s: %v", *message.FileID, err)
			return "", "", &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "failed to refresh file token",
				Type:    "internal_server_error",
			}
		}

		tokenID = newToken.Id
		tokenSecret = newToken.Secret

		// 3. Update database with new token
		tokenExpiry, _ := time.Parse(time.RFC3339, newToken.Expire)
		_ = ps.PersonalQueries.UpdateMessageFileToken(ctx, personal.UpdateMessageFileTokenParams{
			ID:              message.ID,
			FileTokenID:     &tokenID,
			FileTokenSecret: &tokenSecret,
			FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
		})
	} else {
		tokenID = *message.FileTokenID
		tokenSecret = *message.FileTokenSecret
	}

	ad := &utils.AppwriteFileData{
		FileId:     message.FileID,
		FileToken:  &tokenID,
		FileSecret: &tokenSecret,
	}

	var viewURL, downloadURL string

	// Media types get both view and download URLs
	isMedia := message.MessageType == "image" || message.MessageType == "video" || message.MessageType == "audio"
	if !isMedia && message.FileMimeType != nil {
		mime := *message.FileMimeType
		isMedia = strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
	}

	if isMedia {
		viewURLPtr := utils.BuildFileViewURL(ps.Appwrite.Endpoint, ps.Appwrite.ProjectID, ChatFilesBucketID, ad)
		if viewURLPtr != nil {
			viewURL = *viewURLPtr
		}
	}

	// All files get a download URL
	downloadURLPtr := utils.BuildFileDownloadURL(ps.Appwrite.Endpoint, ps.Appwrite.ProjectID, ChatFilesBucketID, ad)
	if downloadURLPtr != nil {
		downloadURL = *downloadURLPtr
	}

	return viewURL, downloadURL, nil
}

