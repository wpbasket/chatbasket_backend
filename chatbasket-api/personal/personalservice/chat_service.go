package personalservice

import (
	"chatbasket-api/internal/db/personal"
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"chatbasket-api/utils"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

const (
	EligibilityAllowed            = "allowed"
	EligibilityNotInContacts      = "not_in_contacts"
	EligibilityRecipientPrivate   = "recipient_private"
	EligibilityBlocked            = "blocked" // Legacy, keep for safety
	EligibilityBlockedByRecipient = "blocked_by_recipient"
	EligibilityBlockedByMe        = "blocked_by_me"
	EligibilityAdminBlocked       = "admin_blocked"
	EligibilityNoPrimaryDevice    = "no_primary_device"
	EligibilityRecipientNotFound  = "recipient_not_found"
)

const (
	DefaultMessageTTL   = 30 * 24 * time.Hour
	StorageFullTTL      = 7 * 24 * time.Hour
	MaxDeliveryAttempts = 5
)

func messagingEligibilityError(eligibility string) *model.ApiError {
	errType := "messaging_not_allowed"
	switch eligibility {
	case EligibilityNotInContacts:
		errType = "messaging_not_allowed_not_in_contacts"
	case EligibilityRecipientPrivate:
		errType = "messaging_not_allowed_recipient_private"
	case EligibilityBlocked, EligibilityBlockedByRecipient:
		errType = "messaging_not_allowed_blocked_by_recipient"
	case EligibilityBlockedByMe:
		errType = "messaging_not_allowed_blocked_by_me"
	case EligibilityAdminBlocked:
		errType = "messaging_not_allowed_admin_blocked"
	case EligibilityNoPrimaryDevice:
		errType = "messaging_not_allowed_no_primary_device"
	case EligibilityRecipientNotFound:
		errType = "messaging_not_allowed_recipient_not_found"
	}

	return &model.ApiError{
		Code:    http.StatusForbidden,
		Message: eligibility,
		Type:    errType,
	}
}

func (ps *Service) CreateOrGetChat(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal.Chat, *model.ApiError) {
	chatID := uuid.New()

	chat, err := ps.PersonalQueries.CreateChat(ctx, personal.CreateChatParams{
		ID:      chatID,
		Column2: user1ID,
		Column3: user2ID,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "chat_creation_failed",
		}
	}

	return &chat, nil
}

func (ps *Service) CheckMessagingEligibility(ctx context.Context, senderID, recipientID uuid.UUID) (string, *model.ApiError) {
	fmt.Printf("[DEBUG] CheckMessagingEligibility: sender=%s, recipient=%s\n", senderID, recipientID)

	status, err := ps.PersonalQueries.CanSendMessage(ctx, personal.CanSendMessageParams{
		Column1: senderID,
		Column2: recipientID,
	})

	if err != nil {
		fmt.Printf("[DEBUG] CanSendMessage query error: %v\n", err)
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "eligibility_check_failed",
		}
	}

	fmt.Printf("[DEBUG] CanSendMessage status: %s\n", status)

	if status != EligibilityAllowed {
		return status, nil
	}

	senderPrimary, err := ps.AuthQueries.GetUserPrimarySession(ctx, senderID)
	if err != nil {
		if err == pgx.ErrNoRows {
			fmt.Printf("[DEBUG] Sender has no primary device\n")
			return EligibilityNoPrimaryDevice, nil
		}
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to check sender primary device",
			Type:    "internal_server_error",
		}
	}

	if senderPrimary.ID == uuid.Nil {
		fmt.Printf("[DEBUG] Sender primary device is nil\n")
		return EligibilityNoPrimaryDevice, nil
	}

	recipientPrimary, err := ps.AuthQueries.GetUserPrimarySession(ctx, recipientID)
	if err != nil {
		if err == pgx.ErrNoRows {
			fmt.Printf("[DEBUG] Recipient has no primary device\n")
			return EligibilityNoPrimaryDevice, nil
		}
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to check recipient primary device",
			Type:    "internal_server_error",
		}
	}

	if recipientPrimary.ID == uuid.Nil {
		fmt.Printf("[DEBUG] Recipient primary device is nil\n")
		return EligibilityNoPrimaryDevice, nil
	}

	fmt.Printf("[DEBUG] All checks passed - allowed\n")
	return EligibilityAllowed, nil
}

type SendMessageParams struct {
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Content     string
	MessageType string
	IsPrimary   bool
}

func (ps *Service) SendMessage(ctx context.Context, params SendMessageParams) (*personal.Message, *model.ApiError) {
	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, apiErr := ps.CreateOrGetChat(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	messageID := uuid.New()
	expiresAt := time.Now().Add(DefaultMessageTTL)

	message, err := ps.PersonalQueries.CreateMessage(ctx, personal.CreateMessageParams{
		ID:                          messageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID,
		RecipientID:                 params.RecipientID,
		Content:                     params.Content,
		MessageType:                 params.MessageType,
		ExpiresAt:                   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: new(bool),
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "message_send_failed",
		}
	}

	// Update chat status (Last Message + Unread Count)
	// We ignore error here to not fail the request if the message was sent successfully
	// but we should log it.
	_ = ps.PersonalQueries.UpdateChatStatus(ctx, personal.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: new(message.Content),
		LastMessageCreatedAt: message.CreatedAt,
		P1LastMessageType:    new(message.MessageType),
		LastMessageSenderID:  pgtype.UUID{Bytes: message.SenderID, Valid: true},
		LastMessageID:        pgtype.UUID{Bytes: message.ID, Valid: true},
	})

	return &message, nil
}

func (ps *Service) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string, sessionId string, userID uuid.UUID) *model.ApiError {
	var err error

	// 1. Fetch message details first (needed for ownership check and chat_id)
	message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "Message not found: " + messageID.String(),
			Type:    "not_found",
		}
	}

	debugLog := log.New(os.Stderr, "[DEBUG-ACK] ", log.LstdFlags)
	// 🛡️ Partial Security Check: Check if session is Primary (Central)
	// We need this to determine if we should mark as "primary delivered" or just "delivered"
	isCentral, apiErr := ps.GlobalService.AuthService.IsSessionCentral(ctx, userID, sessionId)
	debugLog.Printf("IsSessionCentral result: isCentral=%v, userID=%s, sessionId=%s", isCentral, userID, sessionId)
	if apiErr != nil {
		debugLog.Printf("IsSessionCentral ERROR: %v", apiErr)
		if apiErr.Code == http.StatusUnauthorized {
			return &model.ApiError{
				Code:    http.StatusForbidden,
				Message: "Forbidden: Session not found or invalid",
				Type:    "session_invalid",
			}
		}
		return apiErr
	}

	if acknowledgedBy == "recipient" {
		debugLog.Printf("Processing recipient ACK for message %s", messageID)
		// 1. Basic Delivery: ANY device (Primary or Secondary) can ACK that it received the message.
		// This updates delivered_to_recipient = TRUE (if not already).
		err = ps.PersonalQueries.MarkMessageDeliveredToRecipient(ctx, messageID)
		if err != nil {
			debugLog.Printf("MarkMessageDeliveredToRecipient ERROR: %v", err)
			return &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: utils.GetPostgresError(err).Message,
				Type:    "ack_failed_basic",
			}
		}

		// ✨ NEW: Update persistent chat delivery timestamp
		// This is the source of truth for the "Double Grey Tick" (delivered status).
		// We use the message's creation time as the last delivered marker.
		debugLog.Printf("Updating persistent chat %s delivery timestamp to %v for participant %s", message.ChatID, message.CreatedAt.Time, userID)
		err = ps.PersonalQueries.UpdateChatLastDeliveredAt(ctx, personal.UpdateChatLastDeliveredAtParams{
			ChatID:        message.ChatID,
			ParticipantID: userID, // The one who received it
			LastDeliveredAt: pgtype.Timestamptz{
				Time:  message.CreatedAt.Time,
				Valid: true,
			},
		})
		if err != nil {
			debugLog.Printf("UpdateChatLastDeliveredAt ERROR: %v", err)
		}

		// 2. Primary Delivery: ONLY Primary device updates the strict delivery status.
		// This triggers the "consumption" logic required for deletion.
		if isCentral {
			debugLog.Printf("Session is CENTRAL. Marking message %s as Primary Delivered.", messageID)
			err = ps.PersonalQueries.MarkMessageDeliveredToRecipientPrimary(ctx, messageID)
			if err != nil {
				debugLog.Printf("MarkMessageDeliveredToRecipientPrimary ERROR: %v", err)
			}

			// ⚡ Sequential ACK: Also mark all OLDER text messages as Primary Delivered.
			// This prevents relay bloat by ensuring gaps are filled when a newer message is received.
			// Only applies to 'text' messages as per user request.
			debugLog.Printf("Executing Sequential ACK for older messages in chat %s up to %v", message.ChatID, message.CreatedAt)
			err = ps.PersonalQueries.MarkOlderMessagesAsDeliveredToRecipientPrimary(ctx, personal.MarkOlderMessagesAsDeliveredToRecipientPrimaryParams{
				ChatID:      message.ChatID,
				RecipientID: userID,
				CreatedAt:   message.CreatedAt,
			})
			if err != nil {
				debugLog.Printf("MarkOlderMessagesAsDeliveredToRecipientPrimary ERROR: %v", err)
			}
		} else {
			debugLog.Printf("Session is NOT CENTRAL. Skipping Primary Delivery update.")
		}

	} else {
		// Sender Sync ACK
		debugLog.Printf("Processing sender sync ACK for message %s", messageID)
		// 🛡️ STRICT Security: ONLY Primary device can MARK as synced to sender
		if !isCentral {
			debugLog.Printf("REJECTED: Sender sync ACK from non-central device.")
			return &model.ApiError{
				Code:    http.StatusForbidden,
				Message: "Forbidden: Only primary device can ACK sender sync",
				Type:    "forbidden",
			}
		}

		// 🛡️ Verify Ownership: Ensure the authenticated user IS the sender
		if message.SenderID != userID {
			debugLog.Printf("REJECTED: Ownership mismatch. message.SenderID=%s, userID=%s", message.SenderID, userID)
			return &model.ApiError{
				Code:    http.StatusForbidden,
				Message: "Forbidden: You are not the sender of this message",
				Type:    "forbidden",
			}
		}

		err = ps.PersonalQueries.MarkMessageSyncedToSenderPrimary(ctx, messageID)
		if err != nil {
			debugLog.Printf("MarkMessageSyncedToSenderPrimary ERROR: %v", err)
		}
	}

	if err != nil {
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "ack_failed",
		}
	}

	// 2. Per-Message Relay Cleanup: Check if THIS specific message is now eligible for deletion
	//    A message is eligible when BOTH flags are TRUE:
	//    - delivered_to_recipient_primary = TRUE (recipient's primary device confirmed)
	//    - synced_to_sender_primary = TRUE (sender's primary device confirmed)

	// Re-fetch the message to get updated flags
	updatedMessage, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err == nil {
		// Check if both flags are set to TRUE
		// DeliveredToRecipientPrimary is *bool, SyncedToSenderPrimary is bool
		recipientPrimaryDelivered := updatedMessage.DeliveredToRecipientPrimary != nil && *updatedMessage.DeliveredToRecipientPrimary
		senderPrimarySynced := updatedMessage.SyncedToSenderPrimary

		if recipientPrimaryDelivered && senderPrimarySynced {
			// Both sides have confirmed - safe to delete from relay
			ps.deleteMessageFromRelay(ctx, updatedMessage)
		}

		// ⚡ Sequential Cleanup: Also proactively clear any older messages that are now fully confirmed.
		// This handles cases where older ACKs might have completed the "pair" of flags, but didn't trigger deletion.
		debugLog.Printf("Executing Sequential Bulk Cleanup for older messages in chat %s up to %v", updatedMessage.ChatID, updatedMessage.CreatedAt)
		err = ps.PersonalQueries.CleanupOlderFullyAcknowledgedMessages(ctx, personal.CleanupOlderFullyAcknowledgedMessagesParams{
			ChatID:    updatedMessage.ChatID,
			CreatedAt: updatedMessage.CreatedAt,
		})
		if err != nil {
			debugLog.Printf("CleanupOlderFullyAcknowledgedMessages ERROR: %v", err)
		}
	}

	return nil
}

func (ps *Service) deleteMessageFromRelay(ctx context.Context, message personal.Message) {
	messageID := message.ID
	log.Printf("[Relay-Cleanup] Message %s fully delivered and synced. Deleting from server.", messageID)

	// Capture file info before deleting message row
	fileID := ""
	if message.FileID != nil {
		fileID = *message.FileID
	}
	thumbID := ""
	if message.ThumbnailFileID != nil {
		thumbID = *message.ThumbnailFileID
	}

	// Hard delete from messages table
	err := ps.PersonalQueries.DeleteMessage(ctx, messageID)
	if err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to delete row for %s: %v", messageID, err)
		return
	}

	// Async cleanup of files
	if fileID != "" || thumbID != "" {
		go func(fID, tID string) {
			if fID != "" {
				ps.DeleteChatFile(context.Background(), fID)
			}
			if tID != "" {
				ps.DeleteChatFile(context.Background(), tID)
			}
		}(fileID, thumbID)
	}
}

func (ps *Service) UnsendMessage(ctx context.Context, chatID uuid.UUID, messageIDs []uuid.UUID, senderID uuid.UUID, isPrimary bool) *model.ApiError {
	log.Printf("[UnsendMessage] START: Processing %d messages for sender %s in chat %s (isPrimary=%v)", len(messageIDs), senderID, chatID, isPrimary)

	// Start transaction using the Global DB pool
	tx, err := ps.DB.Begin(ctx)
	if err != nil {
		log.Printf("[UnsendMessage] ERROR: Failed to begin transaction: %v", err)
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to start unsend transaction",
			Type:    "internal_server_error",
		}
	}
	defer tx.Rollback(ctx)

	qtx := ps.PersonalQueries.WithTx(tx)

	// Fetch chat details for fallback routing (who is the other participant?)
	chat, err := qtx.GetChatByID(ctx, chatID)
	if err != nil {
		log.Printf("[UnsendMessage] ERROR: Chat %s not found: %v", chatID, err)
		return &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "chat not found",
			Type:    "not_found",
		}
	}

	// Identify recipient from chat participants
	var recipientID uuid.UUID
	if chat.Participant1ID == senderID {
		recipientID = chat.Participant2ID
	} else if chat.Participant2ID == senderID {
		recipientID = chat.Participant1ID
	} else {
		log.Printf("[UnsendMessage] ERROR: User %s is not a participant in chat %s", senderID, chatID)
		return &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "unauthorized: not a participant in this chat",
			Type:    "forbidden",
		}
	}

	messagesToUnsend := make([]personal.Message, 0, len(messageIDs))

	for _, msgID := range messageIDs {
		msg, err := qtx.GetMessageByID(ctx, msgID)
		if err != nil {
			// FALLBACK: If message is not found, it likely was already deleted after delivery.
			// We still want to broadcast the Unsend sync action to ensure the recipient deletes their local copy.
			log.Printf("[UnsendMessage] Message %s not found (likely already deleted or tombstoned). Falling back to sync-only revocation.", msgID)

			// 1. Notify Sender devices (ONLY if action initiated by Secondary)
			if !isPrimary {
				log.Printf("[UnsendMessage] [Fallback] Initiated by Secondary. Creating sync action for Sender.")
				senderPayload, _ := json.Marshal(personalmodel.SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
				_, _ = qtx.CreateSyncAction(ctx, personal.CreateSyncActionParams{
					ID:         uuid.New(),
					UserID:     senderID,
					ActionType: "unsend",
					Payload:    senderPayload,
				})
			}

			// 2. Notify all Recipient devices
			log.Printf("[UnsendMessage] [Fallback] Creating sync action for Recipient %s.", recipientID)
			recipientPayload, _ := json.Marshal(personalmodel.SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
			_, _ = qtx.CreateSyncAction(ctx, personal.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     recipientID,
				ActionType: "unsend",
				Payload:    recipientPayload,
			})
		} else {
			// Security: Only sender can unsend
			if msg.SenderID != senderID {
				log.Printf("[UnsendMessage] ERROR: Unauthorized unsend attempt for msg %s by user %s", msgID, senderID)
				return &model.ApiError{
					Code:    http.StatusForbidden,
					Message: "unauthorized: you can only unsend your own messages",
					Type:    "forbidden",
				}
			}

			// Prevent duplicate unsend
			if msg.MessageType == "unsent" {
				log.Printf("[UnsendMessage] Message %s is already unsent. Skipping.", msgID)
				continue
			}

			messagesToUnsend = append(messagesToUnsend, msg)

			// Soft delete from relay (Tombstone)
			if err := qtx.UpdateMessageToUnsent(ctx, msg.ID); err != nil {
				log.Printf("[UnsendMessage] ERROR: Failed to create tombstone for msg %s: %v", msg.ID, err)
				return &model.ApiError{
					Code:    http.StatusInternalServerError,
					Message: "failed to create message tombstone",
					Type:    "server_error",
				}
			}

			// Notify Sender devices (Primary)
			if !isPrimary {
				senderPayload, _ := json.Marshal(personalmodel.SyncActionPayload{MessageIDs: []string{msg.ID.String()}, ChatID: msg.ChatID.String()})
				_, _ = qtx.CreateSyncAction(ctx, personal.CreateSyncActionParams{
					ID:         uuid.New(),
					UserID:     msg.SenderID,
					ActionType: "unsend",
					Payload:    senderPayload,
				})
			}

			// Notify Recipient devices
			recipientPayload, _ := json.Marshal(personalmodel.SyncActionPayload{MessageIDs: []string{msg.ID.String()}, ChatID: msg.ChatID.String()})
			_, _ = qtx.CreateSyncAction(ctx, personal.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     msg.RecipientID,
				ActionType: "unsend",
				Payload:    recipientPayload,
			})
		}

		// ALWAYS attempt to update Chat Status (Preview/Unread) for EACH msgID being unsent.
		// The SQL queries 'UpdateChatUnsendPreview' and 'UpdateChatUnsendDecrement'
		// now both contain 'AND last_message_id = ...', so they will ONLY apply
		// if this specific message is the one shown in the chat list.
		_ = qtx.UpdateChatUnsendPreview(ctx, personal.UpdateChatUnsendPreviewParams{
			ID:            chatID,
			LastMessageID: pgtype.UUID{Bytes: msgID, Valid: true},
		})

		_ = qtx.UpdateChatUnsendDecrement(ctx, personal.UpdateChatUnsendDecrementParams{
			ID:          chatID,
			RecipientID: recipientID,
			Amount:      1,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("[UnsendMessage] ERROR: Transaction commit failed: %v", err)
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to commit unsend transaction",
			Type:    "server_error",
		}
	}

	log.Printf("[UnsendMessage] COMMITTED SUCCESSFULLY.")

	// Cleanup media files asynchronously after successful DB commit
	for _, msg := range messagesToUnsend {
		go func(m personal.Message) {
			if m.FileID != nil {
				log.Printf("[UnsendMessage] Async deleting file %s", *m.FileID)
				ps.DeleteChatFile(context.Background(), *m.FileID)
			}
			if m.ThumbnailFileID != nil {
				log.Printf("[UnsendMessage] Async deleting thumbnail %s", *m.ThumbnailFileID)
				ps.DeleteChatFile(context.Background(), *m.ThumbnailFileID)
			}
		}(msg)
	}

	return nil
}

func (ps *Service) DeleteMessageForMe(ctx context.Context, messageIDs []uuid.UUID, userID uuid.UUID, isPrimary bool) *model.ApiError {
	// Clear the per-participant preview if the deleted message is the current preview.
	// This works for BOTH primary and secondary devices.
	for _, msgID := range messageIDs {
		msg, err := ps.PersonalQueries.GetMessageByID(ctx, msgID)
		if err == nil {
			_ = ps.PersonalQueries.ClearLastMessageForParticipant(ctx, personal.ClearLastMessageForParticipantParams{
				UserID:    userID,
				ChatID:    msg.ChatID,
				MessageID: pgtype.UUID{Bytes: msgID, Valid: true},
			})

			// Mark the message as deleted in the database based on sender/recipient
			if msg.SenderID == userID {
				_ = ps.PersonalQueries.MarkMessageDeletedBySender(ctx, personal.MarkMessageDeletedBySenderParams{
					ID:       msgID,
					SenderID: userID,
				})
			} else if msg.RecipientID == userID {
				_ = ps.PersonalQueries.MarkMessageDeletedByRecipient(ctx, personal.MarkMessageDeletedByRecipientParams{
					ID:          msgID,
					RecipientID: userID,
				})
			}
		} else {
			// Message already deleted (e.g., by relay cleanup). Still try to clear the chat preview.
			chats, chatsErr := ps.PersonalQueries.GetChatsByUserID(ctx, userID)
			if chatsErr == nil {
				for _, chat := range chats {
					if chat.LastMessageID.Bytes == msgID {
						_ = ps.PersonalQueries.ClearLastMessageForParticipant(ctx, personal.ClearLastMessageForParticipantParams{
							UserID:    userID,
							ChatID:    chat.ID,
							MessageID: pgtype.UUID{Bytes: msgID, Valid: true},
						})
						break
					}
				}
			}
		}
	}

	// If initiated by primary, we do nothing on the backend relay (as per requirements)
	// The primary handles local deletion and other secondaries catch up via p2p later.
	if isPrimary {
	} else {
		// If initiated by secondary, we create a sync action to notify the primary
		for _, msgID := range messageIDs {
			payload, _ := json.Marshal(personalmodel.SyncActionPayload{MessageIDs: []string{msgID.String()}})
			_, err := ps.PersonalQueries.CreateSyncAction(ctx, personal.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     userID,
				ActionType: "delete_for_me",
				Payload:    payload,
			})
			if err != nil {
				return &model.ApiError{
					Code:    http.StatusInternalServerError,
					Message: "failed to create sync action for secondary deletion",
					Type:    "server_error",
				}
			}
		}
	}

	// Instant Cleanup Optimization:
	// If the message is already fully "safe" for the other party, wipe it from the relay right away.
	for _, msgID := range messageIDs {
		msg, err := ps.PersonalQueries.GetMessageByID(ctx, msgID)
		if err != nil {
			continue
		}

		shouldDeleteNow := false
		if msg.SenderID == userID {
			if msg.DeliveredToRecipientPrimary != nil && *msg.DeliveredToRecipientPrimary {
				shouldDeleteNow = true
			}
		} else if msg.RecipientID == userID {
			if msg.SyncedToSenderPrimary {
				shouldDeleteNow = true
			}
		}

		if shouldDeleteNow {
			ps.deleteMessageFromRelay(ctx, msg)
		}
	}

	return nil
}

func (ps *Service) GetSyncActions(ctx context.Context, userID uuid.UUID, limit int32) ([]personal.MessageSyncAction, *model.ApiError) {
	actions, err := ps.PersonalQueries.GetPendingSyncActions(ctx, personal.GetPendingSyncActionsParams{
		UserID: userID,
		Limit:  limit,
	})
	if err != nil {
		log.Printf("[SyncEngine] GetPendingSyncActions failed for user %s: %v", userID, err)
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to fetch sync actions",
			Type:    "fetch_failed",
		}
	}
	return actions, nil
}

func (ps *Service) AcknowledgeSyncAction(ctx context.Context, actionID uuid.UUID, isPrimary bool) *model.ApiError {
	// Only the Primary device should "consume" the action (mark it as delivered/deleted).
	// Secondary devices can acknowledge, but it's a no-op on the backend side regarding the 'message_sync_actions' table.
	if !isPrimary {
		log.Printf("[Sync] AcknowledgeSyncAction: Device is SECONDARY. Skipping consumption of action %s.", actionID)
		return nil
	}

	log.Printf("[Sync] AcknowledgeSyncAction: Device is PRIMARY. Consuming (deleting) action %s.", actionID)
	err := ps.PersonalQueries.ConsumeSyncAction(ctx, actionID)
	if err != nil {
		log.Printf("[Sync] Failed to consume (delete) action %s: %v", actionID, err)
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to acknowledge sync action",
			Type:    "ack_failed",
		}
	}
	return nil
}

func (ps *Service) GetPendingMessages(ctx context.Context, userID uuid.UUID, limit int32) ([]personal.Message, *model.ApiError) {
	messages, err := ps.PersonalQueries.GetPendingMessagesForRecipient(ctx, personal.GetPendingMessagesForRecipientParams{
		RecipientID: userID,
		Limit:       limit,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return messages, nil
}

func (ps *Service) GetChatMessages(ctx context.Context, chatID uuid.UUID, userID uuid.UUID, limit, offset int32) ([]personal.Message, *model.ApiError) {
	messages, err := ps.PersonalQueries.GetChatMessages(ctx, personal.GetChatMessagesParams{
		ChatID:   chatID,
		Limit:    limit,
		Offset:   offset,
		SenderID: userID,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return messages, nil
}

func (ps *Service) IsChatParticipant(ctx context.Context, chatID, userID uuid.UUID) (bool, *model.ApiError) {
	isParticipant, err := ps.PersonalQueries.IsChatParticipant(ctx, personal.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})

	if err != nil {
		return false, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "verification_failed",
		}
	}

	return isParticipant, nil
}

func (ps *Service) CleanupExpiredMessages(ctx context.Context) error {
	log.Printf("[CleanupJob] Starting cleanup of expired and orphaned (fully delivered) messages/files")

	// 1. Fetch expired messages that have files (batched to avoid memory issues)
	const batchSize = 100
	for {
		expiredMessages, err := ps.PersonalQueries.GetExpiredMessagesWithFiles(ctx, batchSize)
		if err != nil {
			log.Printf("[CleanupJob] ERROR: Failed to fetch expired messages with files: %v", err)
			break
		}

		if len(expiredMessages) == 0 {
			break
		}

		log.Printf("[CleanupJob] Found %d expired/orphaned messages with files to clean up", len(expiredMessages))

		for _, msg := range expiredMessages {
			// Delete files from Appwrite
			if msg.FileID != nil && *msg.FileID != "" {
				ps.DeleteChatFile(ctx, *msg.FileID)
			}
			if msg.ThumbnailFileID != nil && *msg.ThumbnailFileID != "" {
				ps.DeleteChatFile(ctx, *msg.ThumbnailFileID)
			}

			// Ideally we would delete individual messages here if we want to be safe,
			// but bulk delete below is faster. The files are gone now.
		}

		// If we got exactly batchSize, there might be more
		if len(expiredMessages) < batchSize {
			break
		}
	}

	// 2. Perform bulk database deletion of ALL expired messages
	err := ps.PersonalQueries.DeleteExpiredMessages(ctx)
	if err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete expired messages from DB: %v", err)
		return err
	}

	log.Printf("[CleanupJob] Cleanup of expired/orphaned messages completed successfully")
	return nil
}

func (ps *Service) DropPendingMessagesBetweenUsers(ctx context.Context, user1ID, user2ID uuid.UUID) error {
	return ps.PersonalQueries.DeletePendingMessagesBetweenUsers(ctx, personal.DeletePendingMessagesBetweenUsersParams{
		SenderID:    user1ID,
		RecipientID: user2ID,
	})
}

func (ps *Service) GetUserChats(ctx context.Context, userID uuid.UUID) ([]personal.GetUserChatsRow, *model.ApiError) {
	chats, err := ps.PersonalQueries.GetUserChats(ctx, userID)

	if err != nil {
		log.Printf("[PersonalChat] GetUserChats failed for user %s: %v", userID, err)
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return chats, nil
}

func (ps *Service) GetChatByParticipants(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal.Chat, *model.ApiError) {
	chat, err := ps.PersonalQueries.GetChatByParticipants(ctx, personal.GetChatByParticipantsParams{
		Column1: user1ID,
		Column2: user2ID,
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{
				Code:    http.StatusNotFound,
				Message: "chat not found",
				Type:    "chat_not_found",
			}
		}
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return &chat, nil
}

// New service methods to match handler patterns
func (ps *Service) CheckEligibilityHandler(ctx context.Context, payload *personalmodel.CheckEligibilityPayload, userId model.UserId) (*personalmodel.MessagingEligibilityResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot check eligibility with yourself",
			Type:    "invalid_recipient",
		}
	}

	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	resp := &personalmodel.MessagingEligibilityResponse{
		Allowed: eligibility == EligibilityAllowed,
		Reason:  "",
	}
	if !resp.Allowed {
		resp.Reason = eligibility
	}

	return resp, nil
}

func (ps *Service) CreateChatHandler(ctx context.Context, payload *personalmodel.CreateChatPayload, userId model.UserId) (*personalmodel.ChatResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot create chat with yourself",
			Type:    "invalid_recipient",
		}
	}

	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, apiErr := ps.CreateOrGetChat(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	var otherReadAt, otherDeliveredAt time.Time
	if chat.Participant1ID == userId.UuidUserId {
		otherReadAt = chat.P2LastReadAt.Time
		otherDeliveredAt = chat.P2LastDeliveredAt.Time
	} else {
		otherReadAt = chat.P1LastReadAt.Time
		otherDeliveredAt = chat.P1LastDeliveredAt.Time
	}

	return &personalmodel.ChatResponse{
		ChatID:                   chat.ID.String(),
		OtherUserID:              recipientID.String(),
		CreatedAt:                chat.CreatedAt.Time,
		UpdatedAt:                chat.UpdatedAt.Time,
		OtherUserLastReadAt:      otherReadAt,
		OtherUserLastDeliveredAt: otherDeliveredAt,
		LastMessageIsFromMe:      false,
	}, nil
}

func (ps *Service) SendMessageHandler(ctx context.Context, payload *personalmodel.SendMessagePayload, userId model.UserId, isPrimary bool) (*personalmodel.MessageResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot send message to yourself",
			Type:    "invalid_recipient",
		}
	}

	message, apiErr := ps.SendMessage(ctx, SendMessageParams{
		SenderID:    userId.UuidUserId,
		RecipientID: recipientID,
		Content:     payload.Content,
		MessageType: payload.MessageType,
		IsPrimary:   isPrimary,
	})

	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  message.DeliveredToRecipient,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             message.CreatedAt.Time,
		ExpiresAt:             message.ExpiresAt.Time,
		IsFromMe:              true,
		FileID:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
	}, nil
}

func (ps *Service) GetMessagesHandler(ctx context.Context, payload *personalmodel.GetMessagesPayload, userId model.UserId) (*personalmodel.GetMessagesResponse, *model.ApiError) {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid chat ID",
			Type:    "invalid_chat_id",
		}
	}

	isParticipant, apiErr := ps.IsChatParticipant(ctx, chatID, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	if !isParticipant {
		return nil, &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "not_chat_participant",
			Type:    "chat_access_denied",
		}
	}

	limit := payload.Limit
	if limit == 0 {
		limit = 50
	}
	offset := payload.Offset

	messages, apiErr := ps.GetChatMessages(ctx, chatID, userId.UuidUserId, limit, offset)
	if apiErr != nil {
		return nil, apiErr
	}

	messageResponses := make([]personalmodel.MessageResponse, len(messages))
	for i, msg := range messages {
		viewURL, downloadURL := "", ""
		if msg.FileID != nil && *msg.FileID != "" {
			var apiErr *model.ApiError
			viewURL, downloadURL, apiErr = ps.GenerateMessageFileURLs(ctx, msg, userId.UuidUserId)
			if apiErr != nil {
				log.Printf("[GetMessages] Failed to generate URLs for message %s: %v", msg.ID, apiErr)
			}
		}

		deliveredToRecipientPrimary := false
		if msg.DeliveredToRecipientPrimary != nil {
			deliveredToRecipientPrimary = *msg.DeliveredToRecipientPrimary
		}

		messageResponses[i] = personalmodel.MessageResponse{
			MessageID:                   msg.ID.String(),
			ChatID:                      msg.ChatID.String(),
			IsFromMe:                    msg.SenderID == userId.UuidUserId,
			RecipientID:                 msg.RecipientID.String(),
			Content:                     msg.Content,
			MessageType:                 msg.MessageType,
			DeliveredToRecipient:        msg.DeliveredToRecipient,
			DeliveredToRecipientPrimary: deliveredToRecipientPrimary,
			SyncedToSenderPrimary:       msg.SyncedToSenderPrimary,
			CreatedAt:                   msg.CreatedAt.Time,
			ExpiresAt:                   msg.ExpiresAt.Time,
			FileID:                      msg.FileID,
			FileName:                    msg.FileName,
			FileSize:                    msg.FileSize,
			FileMimeType:                msg.FileMimeType,
			ViewURL:                     viewURL,
			DownloadURL:                 downloadURL,
		}
	}

	// Fetch other user's last read/delivered from chat
	chat, _ := ps.PersonalQueries.GetChatByID(ctx, chatID)
	otherUserLastReadAt := time.Time{}
	otherUserLastDeliveredAt := time.Time{}
	if chat.ID != uuid.Nil {
		if chat.Participant1ID == userId.UuidUserId {
			otherUserLastReadAt = chat.P2LastReadAt.Time
			otherUserLastDeliveredAt = chat.P2LastDeliveredAt.Time
		} else {
			otherUserLastReadAt = chat.P1LastReadAt.Time
			otherUserLastDeliveredAt = chat.P1LastDeliveredAt.Time
		}
	}

	return &personalmodel.GetMessagesResponse{
		Messages:                 messageResponses,
		Count:                    len(messageResponses),
		OtherUserLastReadAt:      otherUserLastReadAt,
		OtherUserLastDeliveredAt: otherUserLastDeliveredAt,
	}, nil
}

func (ps *Service) AcknowledgeDeliveryHandler(ctx context.Context, payload *personalmodel.AcknowledgeDeliveryPayload, userId model.UserId, sessionId string) (*personalmodel.AcknowledgeDeliveryResponse, *model.ApiError) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid message ID",
			Type:    "invalid_message_id",
		}
	}

	// Check if success is false, return early without processing
	if !payload.Success {
		return &personalmodel.AcknowledgeDeliveryResponse{
			Acknowledged: false,
		}, nil
	}

	// Pass sessionId and userID to AcknowledgeDelivery for internal isPrimary check
	apiErr := ps.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, sessionId, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.AcknowledgeDeliveryResponse{
		Acknowledged: true,
	}, nil
}

func (ps *Service) GetUserChatsHandler(ctx context.Context, userId model.UserId) (*personalmodel.GetUserChatsResponse, *model.ApiError) {
	chats, apiErr := ps.GetUserChats(ctx, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	// Helper for privacy logic (same as in contact_service.go)
	shouldExposeAvatar := func(globalRestrictProfile, exceptionGlobalProfile, globalRestrictAvatar, exceptionGlobalAvatar, userRestrictProfile, userRestrictAvatar bool) bool {
		if globalRestrictProfile {
			return exceptionGlobalProfile
		}
		if globalRestrictAvatar {
			return exceptionGlobalAvatar
		}
		if userRestrictProfile {
			return false
		}
		if userRestrictAvatar {
			return false
		}
		return true
	}

	chatResponses := make([]personalmodel.ChatResponse, len(chats))
	for i, chat := range chats {
		var avatarURL *string
		if shouldExposeAvatar(chat.GlobalRestrictProfile, chat.ExceptionGlobalProfile, chat.GlobalRestrictAvatar, chat.ExceptionGlobalAvatar, chat.UserRestrictProfile, chat.UserRestrictAvatar) {
			url, apiErr := ps.buildAvatarURL(ctx, chat.AvatarFileID, chat.AvatarTokenID, chat.AvatarTokenSecret, chat.AvatarTokenExpiry, chat.OtherUserID)
			if apiErr != nil {
				return nil, apiErr
			}
			avatarURL = url
		}

		var lastMessageContent *string
		var lastMessageCreatedAt *time.Time
		var lastMessageType *string
		var lastMessageSenderID *string

		// LastMessageContent and LastMessageType are interface{} from SQL CASE expressions.
		// We need to type-assert them to string.
		if chat.LastMessageContent != nil {
			if s, ok := chat.LastMessageContent.(string); ok {
				lastMessageContent = &s
			}

			if chat.LastMessageCreatedAt.Valid {
				lastMessageCreatedAt = new(chat.LastMessageCreatedAt.Time)
			}

			if chat.LastMessageType != nil {
				if s, ok := chat.LastMessageType.(string); ok {
					lastMessageType = &s
				}
			}

			if chat.LastMessageSenderID.Valid {
				// Convert [16]byte to slice for FromBytes
				uid, err := uuid.FromBytes(chat.LastMessageSenderID.Bytes[:])
				if err == nil {
					lastMessageSenderID = new(uid.String())
				}
			}
		}

		var otherUserLastReadAt time.Time
		var otherUserLastDeliveredAt time.Time
		if chat.Participant1ID == userId.UuidUserId {
			otherUserLastReadAt = chat.P2LastReadAt.Time
			otherUserLastDeliveredAt = chat.P2LastDeliveredAt.Time
		} else {
			otherUserLastReadAt = chat.P1LastReadAt.Time
			otherUserLastDeliveredAt = chat.P1LastDeliveredAt.Time
		}

		var lastMessageID *string
		if chat.LastMessageID.Valid {
			lastMessageID = new(uuid.Must(uuid.FromBytes(chat.LastMessageID.Bytes[:])).String())
		}

		chatResponses[i] = personalmodel.ChatResponse{
			ChatID:                   chat.ID.String(),
			OtherUserID:              chat.OtherUserID.String(),
			OtherUserName:            chat.OtherUserName,
			OtherUserUsername:        chat.OtherUserUsername,
			AvatarURL:                avatarURL,
			CreatedAt:                chat.CreatedAt.Time,
			UpdatedAt:                chat.UpdatedAt.Time,
			OtherUserLastReadAt:      otherUserLastReadAt,
			OtherUserLastDeliveredAt: otherUserLastDeliveredAt,
			LastMessageContent:       lastMessageContent,
			LastMessageCreatedAt:     lastMessageCreatedAt,
			LastMessageType:          lastMessageType,
			LastMessageSenderID:      lastMessageSenderID,
			LastMessageIsFromMe:      chat.LastMessageSenderID.Valid && uuid.Must(uuid.FromBytes(chat.LastMessageSenderID.Bytes[:])) == userId.UuidUserId,
			LastMessageStatus:        chat.LastMessageStatus,
			LastMessageIsUnsent:      lastMessageType != nil && *lastMessageType == "unsent",
			LastMessageID:            lastMessageID,
			UnreadCount:              int(chat.UnreadCount),
		}
	}

	return &personalmodel.GetUserChatsResponse{
		Chats: chatResponses,
		Count: len(chatResponses),
	}, nil
}

func (ps *Service) UploadFileForMessageHandler(ctx context.Context, c echo.Context, userId model.UserId, isPrimary bool) (*personalmodel.UploadFileResponse, *personalmodel.MessageResponse, *model.ApiError) {
	recipientIDStr := c.FormValue("recipient_id")
	log.Printf("[ChatService] UploadFileForMessageHandler. RecipientID: %s, IsPrimary: %v", recipientIDStr, isPrimary)
	if recipientIDStr == "" {
		return nil, nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "recipient_id is required",
			Type:    "invalid_request",
		}
	}

	recipientID, err := uuid.Parse(recipientIDStr)
	if err != nil {
		return nil, nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot send file to yourself",
			Type:    "invalid_recipient",
		}
	}

	messageType := c.FormValue("message_type")
	if messageType == "" {
		messageType = "file"
	}

	caption := c.FormValue("caption")

	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("[ChatService] No file found in request: %v", err)
		return nil, nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "No file provided",
			Type:    "invalid_request",
		}
	}

	message, apiErr := ps.UploadFileForMessage(ctx, UploadFileForMessageParams{
		SenderID:    userId.UuidUserId,
		RecipientID: recipientID,
		FileHeader:  file,
		MessageType: messageType,
		Caption:     caption,
		IsPrimary:   isPrimary,
	})

	if apiErr != nil {
		return nil, nil, apiErr
	}

	viewURL, downloadURL, apiErr := ps.GenerateMessageFileURLs(ctx, *message, userId.UuidUserId)
	if apiErr != nil {
		return nil, nil, apiErr
	}

	messageResponse := &personalmodel.MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  false,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             message.CreatedAt.Time,
		ExpiresAt:             message.ExpiresAt.Time,
		IsFromMe:              true,
		FileID:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
		ViewURL:               viewURL,
		DownloadURL:           downloadURL,
	}

	return &personalmodel.UploadFileResponse{
		MessageID:    message.ID.String(),
		FileID:       *message.FileID,
		MessageType:  message.MessageType,
		FileMimeType: message.FileMimeType,
		ViewURL:      viewURL,
		DownloadURL:  downloadURL,
		FileName:     message.FileName,
		FileSize:     message.FileSize,
		CreatedAt:    message.CreatedAt.Time,
		ExpiresAt:    message.ExpiresAt.Time,
	}, messageResponse, nil
}

func (ps *Service) GetFileURLHandler(ctx context.Context, payload *personalmodel.GetFileURLPayload, userId model.UserId) (*personalmodel.GetFileURLResponse, *model.ApiError) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid message ID",
			Type:    "invalid_request",
		}
	}

	message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "Message not found",
			Type:    "not_found",
		}
	}

	viewURL, downloadURL, apiErr := ps.GenerateMessageFileURLs(ctx, message, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.GetFileURLResponse{
		ViewURL:     viewURL,
		DownloadURL: downloadURL,
	}, nil
}

func (ps *Service) MarkChatRead(ctx context.Context, userID uuid.UUID, chatID uuid.UUID, isPrimary bool) *model.ApiError {
	// Verify user is participant
	isParticipant, err := ps.PersonalQueries.IsChatParticipant(ctx, personal.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to check chat participation",
			Type:    "server_error",
		}
	}
	if !isParticipant {
		return &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "User is not a participant of this chat",
			Type:    "forbidden",
		}
	}

	// Mark chat as read (updates chat metadata/unread counters only)
	// Read status is separate from delivery ACK - opening a chat does NOT mean all messages were received
	log.Printf("[MarkChatRead] Resetting read status for chat %s, user %s", chatID, userID)
	err = ps.PersonalQueries.ResetChatReadStatus(ctx, personal.ResetChatReadStatusParams{
		ID:             chatID,
		Participant1ID: userID,
	})
	if err != nil {
		log.Printf("[MarkChatRead] ERROR: Failed to reset read status: %v", err)
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to mark chat as read",
			Type:    "server_error",
		}
	}

	// ⚡ Also update persistent delivery timestamp (if it's read, it's delivered)
	_ = ps.PersonalQueries.UpdateChatLastDeliveredAt(ctx, personal.UpdateChatLastDeliveredAtParams{
		ChatID:        chatID,
		ParticipantID: userID,
		LastDeliveredAt: pgtype.Timestamptz{
			Time:  time.Now(),
			Valid: true,
		},
	})

	// 🛡️ Proactive Delivery: If this is a PRIMARY session, mark all messages as primary-delivered.
	// This ensures that simply opening a chat on a primary device counts as "consumption".
	if isPrimary {
		log.Printf("[MarkChatRead] Primary session detected. Marking all messages in chat %s as primary-delivered for user %s", chatID, userID)
		err = ps.PersonalQueries.MarkChatMessagesAsReadPrimary(ctx, personal.MarkChatMessagesAsReadPrimaryParams{
			ChatID:      chatID,
			RecipientID: userID,
		})
		if err != nil {
			log.Printf("[MarkChatRead] ERROR: Failed to mark messages as read primary: %v", err)
			// We don't return error here because the main operation (ResetChatReadStatus) succeeded.
		}
	}

	return nil
}

func (ps *Service) MarkChatReadHandler(ctx context.Context, payload *personalmodel.MarkChatReadPayload, userId model.UserId, isPrimary bool) *model.ApiError {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid chat ID",
			Type:    "invalid_request",
		}
	}

	return ps.MarkChatRead(ctx, userId.UuidUserId, chatID, isPrimary)
}

func (ps *Service) UnsendMessageHandler(ctx context.Context, payload *personalmodel.UnsendMessagePayload, userId model.UserId, isPrimary bool) *model.ApiError {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid chat ID",
			Type:    "invalid_request",
		}
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return &model.ApiError{
				Code:    http.StatusBadRequest,
				Message: "Invalid message ID in list",
				Type:    "invalid_request",
			}
		}
		msgUUIDs = append(msgUUIDs, id)
	}

	return ps.UnsendMessage(ctx, chatID, msgUUIDs, userId.UuidUserId, isPrimary)
}

func (ps *Service) DeleteMessageForMeHandler(ctx context.Context, payload *personalmodel.DeleteMessageForMePayload, userId model.UserId, isPrimary bool) *model.ApiError {
	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return &model.ApiError{
				Code:    http.StatusBadRequest,
				Message: "Invalid message ID in list",
				Type:    "invalid_request",
			}
		}
		msgUUIDs = append(msgUUIDs, id)
	}

	return ps.DeleteMessageForMe(ctx, msgUUIDs, userId.UuidUserId, isPrimary)
}

func (ps *Service) GetSyncActionsHandler(ctx context.Context, payload *personalmodel.GetSyncActionsPayload, userId model.UserId) (*personalmodel.GetSyncActionsResponse, *model.ApiError) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}

	actions, apiErr := ps.GetSyncActions(ctx, userId.UuidUserId, limit)
	if apiErr != nil {
		log.Printf("[PersonalChat] GetSyncActions failed for user %s: %v", userId.UuidUserId, apiErr)
		return nil, apiErr
	}

	respActions := make([]personalmodel.SyncActionResponse, 0, len(actions))
	for _, a := range actions {
		var payloadObj personalmodel.SyncActionPayload
		_ = json.Unmarshal(a.Payload, &payloadObj)

		respActions = append(respActions, personalmodel.SyncActionResponse{
			ID:                 a.ID.String(),
			UserID:             a.UserID.String(),
			ActionType:         a.ActionType,
			Payload:            payloadObj,
			DeliveredToPrimary: a.DeliveredToPrimary,
			CreatedAt:          a.CreatedAt.Time,
		})
	}

	return &personalmodel.GetSyncActionsResponse{
		Actions: respActions,
		Count:   len(respActions),
	}, nil
}

func (ps *Service) AcknowledgeSyncActionHandler(ctx context.Context, payload *personalmodel.AcknowledgeSyncActionPayload, isPrimary bool) *model.ApiError {
	actionID, err := uuid.Parse(payload.ActionID)
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid action ID",
			Type:    "invalid_request",
		}
	}

	return ps.AcknowledgeSyncAction(ctx, actionID, isPrimary)
}

func (ps *Service) GetPendingMessagesHandler(ctx context.Context, payload *personalmodel.GetPendingMessagesPayload, userId model.UserId) (*personalmodel.GetMessagesResponse, *model.ApiError) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}

	// 1. Fetch undelivered messages (where user is recipient)
	messagesRecv, err := ps.PersonalQueries.GetPendingMessagesForRecipient(ctx, personal.GetPendingMessagesForRecipientParams{
		RecipientID: userId.UuidUserId,
		Limit:       limit,
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "internal_server_error",
		}
	}

	// 2. Fetch unsynced messages (where user is sender but primary doesn't have it yet)
	messagesSent, err := ps.PersonalQueries.GetPendingSenderSyncMessages(ctx, personal.GetPendingSenderSyncMessagesParams{
		SenderID: userId.UuidUserId,
		Limit:    limit,
	})
	if err != nil {
		log.Printf("[GetPendingMessages] Warning: Failed to fetch sender-sync messages: %v", err)
	}

	totalCount := len(messagesRecv) + len(messagesSent)
	messageResponses := make([]personalmodel.MessageResponse, 0, totalCount)

	// Helper to map DB message to Response
	mapMsg := func(m personal.Message) personalmodel.MessageResponse {
		viewURL, downloadURL := "", ""
		if m.FileID != nil && *m.FileID != "" {
			var apiErr *model.ApiError
			viewURL, downloadURL, apiErr = ps.GenerateMessageFileURLs(ctx, m, userId.UuidUserId)
			if apiErr != nil {
				log.Printf("[GetPendingMessages] Failed to generate URLs for message %s: %v", m.ID, apiErr)
			}
		}

		deliveredToRecipientPrimary := false
		if m.DeliveredToRecipientPrimary != nil {
			deliveredToRecipientPrimary = *m.DeliveredToRecipientPrimary
		}

		return personalmodel.MessageResponse{
			MessageID:                   m.ID.String(),
			ChatID:                      m.ChatID.String(),
			RecipientID:                 m.RecipientID.String(),
			Content:                     m.Content,
			MessageType:                 m.MessageType,
			DeliveredToRecipient:        m.DeliveredToRecipient,
			DeliveredToRecipientPrimary: deliveredToRecipientPrimary,
			SyncedToSenderPrimary:       m.SyncedToSenderPrimary,
			CreatedAt:                   m.CreatedAt.Time,
			ExpiresAt:                   m.ExpiresAt.Time,
			IsFromMe:                    m.SenderID == userId.UuidUserId,
			FileID:                      m.FileID,
			FileName:                    m.FileName,
			FileSize:                    m.FileSize,
			FileMimeType:                m.FileMimeType,
			ViewURL:                     viewURL,
			DownloadURL:                 downloadURL,
		}
	}

	for _, m := range messagesRecv {
		messageResponses = append(messageResponses, mapMsg(m))
	}
	for _, m := range messagesSent {
		messageResponses = append(messageResponses, mapMsg(m))
	}

	return &personalmodel.GetMessagesResponse{
		Messages: messageResponses,
		Count:    len(messageResponses),
	}, nil
}
