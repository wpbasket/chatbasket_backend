package personal_chat

import (
	"fmt"
	"chatbasket-api/internal/platform/kit"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// WS Event Type Constants
// ──────────────────────────────────────────────────────────────────────────────

const (
	WSEventNewMessage  = "new_message"
	WSEventDeliveryAck = "delivery_ack"
	WSEventReadReceipt = "read_receipt"
	WSEventUnsend      = "unsend"
	WSEventDeleteForMe = "delete_for_me"
	WSEventSyncAction  = "sync_action"
	WSEventHistorySyncRequested = "history_sync_requested"
	WSEventHistorySyncReady     = "history_sync_ready"
)

// ──────────────────────────────────────────────────────────────────────────────
// Service Constants
// ──────────────────────────────────────────────────────────────────────────────

const (
	HistorySyncTTL = 10 * time.Minute
)

const (
	MaxFileSize         = 100 * 1024 * 1024
	DefaultMessageTTL   = 30 * 24 * time.Hour
	StorageFullTTL      = 7 * 24 * time.Hour
	MaxDeliveryAttempts = 5
)

// ──────────────────────────────────────────────────────────────────────────────
// Response Structs
// ──────────────────────────────────────────────────────────────────────────────

type ChatResponse struct {
	ChatID                   string     `json:"chat_id"`
	OtherUserID              string     `json:"other_user_id"`
	OtherUserName            string     `json:"other_user_name"`
	OtherUserUsername        string     `json:"other_user_username"`
	OtherUserBio             *string    `json:"other_user_bio"`
	AvatarURL                *string    `json:"avatar_url"`
	AvatarFileId             *string    `json:"avatar_file_id"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	OtherUserLastReadAt      time.Time  `json:"other_user_last_read_at"`
	OtherUserLastDeliveredAt time.Time  `json:"other_user_last_delivered_at"`
	LastMessageContent       *string    `json:"last_message_content"`
	LastMessageCreatedAt     *time.Time `json:"last_message_created_at"`
	LastMessageType          *string    `json:"last_message_type"`
	LastMessageSenderID      *string    `json:"-"`
	LastMessageIsFromMe      bool       `json:"last_message_is_from_me"`
	LastMessageStatus        string     `json:"last_message_status"`
	LastMessageIsUnsent      bool       `json:"last_message_is_unsent"`
	LastMessageID            *string    `json:"last_message_id"`
	UnreadCount              int        `json:"unread_count"`
	OtherUserKeysRevision    int32      `json:"other_user_keys_revision"`
	ProfileType              string     `json:"profile_type"`
}

type MessageResponse struct {
	MessageID                   string     `json:"message_id"`
	ChatID                      string     `json:"chat_id"`
	RecipientID                 string     `json:"recipient_id"`
	SenderKeysRevision          int32      `json:"sender_keys_revision"`
	Content                     string     `json:"content"`
	MessageType                 string     `json:"message_type"`
	DeliveredToRecipient        bool       `json:"delivered_to_recipient"`
	DeliveredToRecipientPrimary bool       `json:"delivered_to_recipient_primary"`
	SyncedToSenderPrimary       bool       `json:"synced_to_sender_primary"`
	CreatedAt                   time.Time  `json:"created_at"`
	ExpiresAt                   time.Time  `json:"expires_at"`
	IsFromMe                    bool       `json:"is_from_me"`
	FileID                      *string    `json:"file_id"`
	FileName                    *string    `json:"file_name"`
	FileSize                    *int64     `json:"file_size"`
	FileMimeType                *string    `json:"file_mime_type"`
	ViewURL                     string     `json:"view_url,omitempty"`
	DownloadURL                 string     `json:"download_url,omitempty"`
}

type MessagingEligibilityResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type GetMessagesResponse struct {
	Messages                 []MessageResponse `json:"messages"`
	Count                    int               `json:"count"`
	OtherUserLastReadAt      time.Time         `json:"other_user_last_read_at"`
	OtherUserLastDeliveredAt time.Time         `json:"other_user_last_delivered_at"`
}

type GetUserChatsResponse struct {
	Chats []ChatResponse `json:"chats"`
	Count int            `json:"count"`
}

type AcknowledgeDeliveryResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

type AckDeliveryBatchResponse struct {
	AcknowledgedCount int `json:"acknowledged_count"`
}

type GetFileURLResponse struct {
	ViewURL     string `json:"view_url,omitempty"`
	DownloadURL string `json:"download_url"`
}

// PresignChatUploadResponse is returned by POST /chat/presign.
type PresignChatUploadResponse struct {
	FileID       string    `json:"file_id"`
	PresignedURL string    `json:"presigned_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ConfirmChatUploadPayload is the request body for POST /chat/confirm.
type ConfirmChatUploadPayload struct {
	FileID                string `json:"file_id" validate:"required"`
	RecipientID           string `json:"recipient_id" validate:"required,uuid"`
	Content               string `json:"content" validate:"required,max=5000"`
	MessageType           string `json:"message_type" validate:"required,oneof=image video audio file"`
	RecipientKeysRevision int32  `json:"recipient_keys_revision"`
	SenderKeysRevision    int32  `json:"sender_keys_revision"`
}

// ConfirmChatUploadResponse is returned by POST /chat/confirm.
type ConfirmChatUploadResponse struct {
	MessageID          string    `json:"message_id"`
	ChatID             string    `json:"chat_id"`
	RecipientID        string    `json:"recipient_id"`
	SenderKeysRevision int32     `json:"sender_keys_revision"`
	FileID             string    `json:"file_id"`
	MessageType        string    `json:"message_type"`
	ViewURL            string    `json:"view_url,omitempty"`
	DownloadURL        string    `json:"download_url"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type SyncActionResponse struct {
	ID                 string            `json:"id"`
	UserID             string            `json:"user_id"`
	ActionType         string            `json:"action_type"`
	Payload            SyncActionPayload `json:"payload"`
	DeliveredToPrimary bool              `json:"delivered_to_primary"`
	CreatedAt          time.Time         `json:"created_at"`
}

type GetSyncActionsResponse struct {
	Actions []SyncActionResponse `json:"actions"`
	Count   int                  `json:"count"`
}


// ──────────────────────────────────────────────────────────────────────────────
// StaleKeysError — returned when client's keys_revision is out of date
// ──────────────────────────────────────────────────────────────────────────────

// StaleSide indicates which side has stale keys
type StaleSide string

const (
	StaleSideSender    StaleSide = "sender"
	StaleSideRecipient StaleSide = "recipient"
	StaleSideBoth      StaleSide = "both"
)

// StaleKeysErrorDetails carries the fresh keys and revisions for the stale side(s)
type StaleKeysErrorDetails struct {
	StaleSide              StaleSide `json:"stale_side"`
	SenderKeysRevision     int32     `json:"sender_keys_revision,omitempty"`
	RecipientKeysRevision  int32     `json:"recipient_keys_revision,omitempty"`
	SenderActiveKeys       []string  `json:"sender_active_keys,omitempty"`
	RecipientActiveKeys    []string  `json:"recipient_active_keys,omitempty"`
}

// StaleKeysError is a conflict error that carries structured data about which keys are stale.
// Implements kit.DetailedProcessedError so the global error handler includes the details in the JSON response.
type StaleKeysError struct {
	details StaleKeysErrorDetails
}

func NewStaleKeysError(details StaleKeysErrorDetails) *StaleKeysError {
	return &StaleKeysError{details: details}
}

func (e *StaleKeysError) Error() string {
	return fmt.Sprintf("keys_revision is stale (side: %s)", e.details.StaleSide)
}

func (e *StaleKeysError) Status() int {
	return 409 // Conflict
}

func (e *StaleKeysError) Kind() string {
	return "keys_stale"
}

func (e *StaleKeysError) Details() interface{} {
	return e.details
}

// ──────────────────────────────────────────────────────────────────────────────
// Request Payloads
// ──────────────────────────────────────────────────────────────────────────────

type CheckEligibilityPayload struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type CreateChatPayload struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type SendMessagePayload struct {
	RecipientID           string `json:"recipient_id" validate:"required,uuid"`
	Content               string `json:"content" validate:"required,max=5000"`
	MessageType           string `json:"message_type" validate:"required,oneof=text image video audio file"`
	RecipientKeysRevision int32  `json:"recipient_keys_revision"`
	SenderKeysRevision    int32  `json:"sender_keys_revision"`
}

type AcknowledgeDeliveryPayload struct {
	MessageID      string `json:"message_id" validate:"required,uuid"`
	AcknowledgedBy string `json:"acknowledged_by" validate:"required,oneof=recipient sender"`
	Success        bool   `json:"success"`
}

type GetMessagesPayload struct {
	ChatID string `query:"chat_id" validate:"required,uuid"`
	Limit  int32  `query:"limit"`
	Offset int32  `query:"offset"`
}

type MarkChatReadPayload struct {
	ChatID string `json:"chat_id" validate:"required,uuid"`
}

type GetFileURLPayload struct {
	MessageID string `json:"message_id" query:"message_id" validate:"required,uuid"`
}

type UnsendMessagePayload struct {
	ChatID     string   `json:"chat_id" validate:"required,uuid"`
	MessageIDs []string `json:"message_ids" validate:"required,min=1,dive,uuid"`
}

type DeleteMessageForMePayload struct {
	MessageIDs []string `json:"message_ids" validate:"required,min=1,dive,uuid"`
}

type GetSyncActionsPayload struct {
	Limit int32 `query:"limit"`
}

type GetPendingMessagesPayload struct {
	Limit int32 `query:"limit"`
}

type AcknowledgeSyncActionPayload struct {
	ActionID string `json:"action_id" validate:"required,uuid"`
}

type AckDeliveryBatchPayload struct {
	MessageIDs     []string `json:"message_ids" validate:"required,min=1,dive,uuid"`
	AcknowledgedBy string   `json:"acknowledged_by" validate:"required,oneof=recipient sender"`
}

// ──────────────────────────────────────────────────────────────────────────────
// WS Event Payloads
// ──────────────────────────────────────────────────────────────────────────────

type DeliveryAckEventPayload struct {
	ChatID      string    `json:"chat_id"`
	MessageIDs  []string  `json:"message_ids"`
	DeliveredAt time.Time `json:"delivered_at"`
}

type ReadReceiptEventPayload struct {
	ChatID   string `json:"chat_id"`
	ReaderID string `json:"reader_id"`
	ReadAt   string `json:"read_at"`
}

type UnsendEventPayload struct {
	ChatID     string   `json:"chat_id"`
	MessageIDs []string `json:"message_ids"`
	SenderID   string   `json:"sender_id"`
}

type DeleteForMeEventPayload struct {
	ChatID     string   `json:"chat_id"`
	MessageIDs []string `json:"message_ids"`
}

type SyncActionPayload struct {
	MessageIDs []string `json:"message_ids"`
	ChatID     string   `json:"chat_id,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal Param Structs
// ──────────────────────────────────────────────────────────────────────────────

type SendMessageParams struct {
	SenderID              kit.UserId
	RecipientID           uuid.UUID
	Content               string
	MessageType           string
	IsPrimary             bool
	RecipientKeysRevision int32
	SenderKeysRevision    int32
}

