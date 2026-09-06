package personal_chat

import (
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/platform/kit"
	"time"

	"github.com/google/uuid"
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
	ChatID                   string     `json:"chatId"`
	OtherUserID              string     `json:"otherUserId"`
	OtherUserName            string     `json:"otherUserName"`
	OtherUserUsername        string     `json:"otherUserUsername"`
	OtherUserBio             *string    `json:"otherUserBio"`
	AvatarURL                *string    `json:"avatarUrl"`
	AvatarFileId             *string    `json:"avatarFileId"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
	OtherUserLastReadAt      time.Time  `json:"otherUserLastReadAt"`
	OtherUserLastDeliveredAt time.Time  `json:"otherUserLastDeliveredAt"`
	LastMessageContent       *string    `json:"lastMessageContent"`
	LastMessageCreatedAt     *time.Time `json:"lastMessageCreatedAt"`
	LastMessageType          *string    `json:"lastMessageType"`
	LastMessageSenderID      *string    `json:"-"`
	LastMessageIsFromMe      bool       `json:"lastMessageIsFromMe"`
	LastMessageStatus        string     `json:"lastMessageStatus"`
	LastMessageIsUnsent      bool       `json:"lastMessageIsUnsent"`
	LastMessageID            *string    `json:"lastMessageId"`
	UnreadCount              int        `json:"unreadCount"`
	OtherUserKeysRevision    int32      `json:"otherUserKeysRevision"`
	ProfileType              string     `json:"profileType"`
}

type MessageResponse struct {
	MessageID                   string     `json:"messageId"`
	ChatID                      string     `json:"chatId"`
	RecipientID                 string     `json:"recipientId"`
	SenderKeysRevision          int32      `json:"senderKeysRevision"`
	Content                     string     `json:"content"`
	MessageType                 string     `json:"messageType"`
	DeliveredToRecipient        bool       `json:"deliveredToRecipient"`
	DeliveredToRecipientPrimary bool       `json:"deliveredToRecipientPrimary"`
	SyncedToSenderPrimary       bool       `json:"syncedToSenderPrimary"`
	CreatedAt                   time.Time  `json:"createdAt"`
	ExpiresAt                   time.Time  `json:"expiresAt"`
	IsFromMe                    bool       `json:"isFromMe"`
	FileID                      *string    `json:"fileId"`
	FileName                    *string    `json:"fileName"`
	FileSize                    *int64     `json:"fileSize"`
	FileMimeType                *string    `json:"fileMimeType"`
	ViewURL                     string     `json:"viewUrl,omitempty"`
	DownloadURL                 string     `json:"downloadUrl,omitempty"`
	ReadByRecipient             bool       `json:"readByRecipient"`
	ReadAckedBySender           bool       `json:"readAckedBySender"`
	ReadAt                      *time.Time `json:"readAt,omitempty"`
	IsConsumed                  bool       `json:"isConsumed"`
}

type MessagingEligibilityResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type GetMessagesResponse struct {
	Messages                 []MessageResponse `json:"messages"`
	Count                    int               `json:"count"`
	OtherUserLastReadAt      time.Time         `json:"otherUserLastReadAt"`
	OtherUserLastDeliveredAt time.Time         `json:"otherUserLastDeliveredAt"`
}

type GetUserChatsResponse struct {
	Chats []ChatResponse `json:"chats"`
	Count int            `json:"count"`
}

type AcknowledgeDeliveryResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

type AckDeliveryBatchResponse struct {
	AcknowledgedCount      int      `json:"acknowledgedCount"`
	AcknowledgedMessageIds []string `json:"acknowledgedMessageIds"`
}

type AckReadReceiptBatchResponse struct {
	AcknowledgedCount      int      `json:"acknowledgedCount"`
	AcknowledgedMessageIds []string `json:"acknowledgedMessageIds"`
}

type MarkChatReadResponse struct {
	Status       bool                              `json:"status"`
	ReadAt       string                            `json:"readAt"`
	ReadMessages []*rpc_personal_chatv1.MessageReadReceipt `json:"readMessages"`
}

type GetFileURLResponse struct {
	ViewURL     string `json:"viewUrl,omitempty"`
	DownloadURL string `json:"downloadUrl"`
}

// PresignChatUploadResponse is returned by POST /chat/presign.
type PresignChatUploadResponse struct {
	FileID       string    `json:"fileId"`
	PresignedURL string    `json:"presignedUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// ConfirmChatUploadPayload is the request body for POST /chat/confirm.
type ConfirmChatUploadPayload struct {
	MessageID             string `json:"messageId" validate:"required,uuid"`
	FileID                string `json:"fileId" validate:"required"`
	RecipientID           string `json:"recipientId" validate:"required,uuid"`
	Content               string `json:"content" validate:"required,max=5000"`
	MessageType           string `json:"messageType" validate:"required,oneof=image video audio file"`
	RecipientKeysRevision int32  `json:"recipientKeysRevision"`
	SenderKeysRevision    int32  `json:"senderKeysRevision"`
}

// ConfirmChatUploadResponse is returned by POST /chat/confirm.
type ConfirmChatUploadResponse struct {
	MessageID          string    `json:"messageId"`
	ChatID             string    `json:"chatId"`
	RecipientID        string    `json:"recipientId"`
	SenderKeysRevision int32     `json:"senderKeysRevision"`
	FileID             string    `json:"fileId"`
	MessageType        string    `json:"messageType"`
	ViewURL            string    `json:"viewUrl,omitempty"`
	DownloadURL        string    `json:"downloadUrl"`
	CreatedAt          time.Time `json:"createdAt"`
	ExpiresAt          time.Time `json:"expiresAt"`
}

type SyncActionResponse struct {
	ID                 string            `json:"id"`
	UserID             string            `json:"userId"`
	ActionType         string            `json:"actionType"`
	Payload            SyncActionPayload `json:"payload"`
	DeliveredToPrimary bool              `json:"deliveredToPrimary"`
	CreatedAt          time.Time         `json:"createdAt"`
}

type GetSyncActionsResponse struct {
	Actions []SyncActionResponse `json:"actions"`
	Count   int                  `json:"count"`
}


// ──────────────────────────────────────────────────────────────────────────────
// Stale keys error — returned when client's keys_revision is out of date.
// The details are passed to kit.NewErrorWithDetails in the chat service.
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
	StaleSide              StaleSide `json:"staleSide"`
	SenderKeysRevision     int32     `json:"senderKeysRevision,omitempty"`
	RecipientKeysRevision  int32     `json:"recipientKeysRevision,omitempty"`
	SenderActiveKeys       []string  `json:"senderActiveKeys,omitempty"`
	RecipientActiveKeys    []string  `json:"recipientActiveKeys,omitempty"`
}


// ──────────────────────────────────────────────────────────────────────────────
// Request Payloads
// ──────────────────────────────────────────────────────────────────────────────

type CheckEligibilityPayload struct {
	RecipientID string `json:"recipientId" validate:"required,uuid"`
}

type CreateChatPayload struct {
	RecipientID string `json:"recipientId" validate:"required,uuid"`
}

type SendMessagePayload struct {
	MessageID             string `json:"messageId" validate:"required,uuid"`
	RecipientID           string `json:"recipientId" validate:"required,uuid"`
	Content               string `json:"content" validate:"required,max=5000"`
	MessageType           string `json:"messageType" validate:"required,oneof=text image video audio file"`
	RecipientKeysRevision int32  `json:"recipientKeysRevision"`
	SenderKeysRevision    int32  `json:"senderKeysRevision"`
}

type AcknowledgeDeliveryPayload struct {
	MessageID      string `json:"messageId" validate:"required,uuid"`
	AcknowledgedBy string `json:"acknowledgedBy" validate:"required,oneof=recipient sender"`
	Success        bool   `json:"success"`
}

type GetMessagesPayload struct {
	ChatID         string     `query:"chatId" validate:"required,uuid"`
	Limit          int32      `query:"limit"`
	AfterCreatedAt *time.Time `query:"afterCreatedAt"`
	AfterMessageID *string    `query:"afterMessageId" validate:"omitempty,uuid"`
}

type AckAndReadBatchPayload struct {
	MessageIDs []string `json:"messageIds" validate:"required,min=1,dive,uuid"`
}

type MarkChatReadPayload struct {
	ChatID     string   `json:"chatId" validate:"required,uuid"`
	MessageIDs []string `json:"messageIds" validate:"omitempty,dive,uuid"`
}

type GetFileURLPayload struct {
	MessageID string `json:"messageId" query:"messageId" validate:"required,uuid"`
}

type UnsendMessagePayload struct {
	ChatID     string   `json:"chatId" validate:"required,uuid"`
	MessageIDs []string `json:"messageIds" validate:"required,min=1,dive,uuid"`
}

type DeleteMessageForMePayload struct {
	MessageIDs []string `json:"messageIds" validate:"required,min=1,dive,uuid"`
}

type GetSyncActionsPayload struct {
	Limit int32 `query:"limit"`
}

type GetPendingMessagesPayload struct {
	Limit                   int32      `query:"limit"`
	AfterRecipientCreatedAt *time.Time `query:"afterRecipientCreatedAt"`
	AfterRecipientMessageID *string    `query:"afterRecipientMessageId"`
	AfterSenderCreatedAt    *time.Time `query:"afterSenderCreatedAt"`
	AfterSenderMessageID    *string    `query:"afterSenderMessageId"`
}

type AcknowledgeSyncActionPayload struct {
	ActionID string `json:"actionId" validate:"required,uuid"`
}

type AckDeliveryBatchPayload struct {
	MessageIDs     []string `json:"messageIds" validate:"required,min=1,dive,uuid"`
	AcknowledgedBy string   `json:"acknowledgedBy" validate:"required,oneof=recipient sender"`
}

type AckReadReceiptBatchPayload struct {
	ChatID     string   `json:"chatId" validate:"required,uuid"`
	MessageIDs []string `json:"messageIds" validate:"required,min=1,dive,uuid"`
}

// ──────────────────────────────────────────────────────────────────────────────
// WS Event Payloads
// ──────────────────────────────────────────────────────────────────────────────

type DeliveryAckEventPayload struct {
	ChatID      string    `json:"chatId"`
	MessageIDs  []string  `json:"messageIds"`
	DeliveredAt time.Time `json:"deliveredAt"`
}

type ReadReceiptEventPayload struct {
	ChatID   string `json:"chatId"`
	ReaderID string `json:"readerId"`
	ReadAt   string `json:"readAt"`
}

type UnsendEventPayload struct {
	ChatID     string   `json:"chatId"`
	MessageIDs []string `json:"messageIds"`
	SenderID   string   `json:"senderId"`
}

type DeleteForMeEventPayload struct {
	ChatID     string   `json:"chatId"`
	MessageIDs []string `json:"messageIds"`
}

type SyncActionPayload struct {
	MessageIDs []string `json:"messageIds"`
	ChatID     string   `json:"chatId,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal Param Structs
// ──────────────────────────────────────────────────────────────────────────────

type SendMessageParams struct {
	MessageID             uuid.UUID
	SenderID              kit.UserId
	RecipientID           uuid.UUID
	Content               string
	MessageType           string
	IsPrimary             bool
	RecipientKeysRevision int32
	SenderKeysRevision    int32
}

