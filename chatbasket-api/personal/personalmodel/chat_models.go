package personalmodel

import (
	"time"
)

type ChatResponse struct {
	ChatID               string     `json:"chat_id"`
	OtherUserID          string     `json:"other_user_id"`
	OtherUserName        string     `json:"other_user_name"`
	OtherUserUsername    string     `json:"other_user_username"`
	AvatarURL            *string    `json:"avatar_url"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	OtherUserLastReadAt  time.Time  `json:"other_user_last_read_at"`
	LastMessageContent   *string    `json:"last_message_content"`
	LastMessageCreatedAt *time.Time `json:"last_message_created_at"`
	LastMessageType      *string    `json:"last_message_type"`
	LastMessageSenderID  *string    `json:"-"`
	LastMessageIsFromMe  bool       `json:"last_message_is_from_me"`
	LastMessageStatus    string     `json:"last_message_status"`
	LastMessageIsUnsent  bool       `json:"last_message_is_unsent"`
	LastMessageID        *string    `json:"last_message_id"`
	UnreadCount          int        `json:"unread_count"`
}

type MessageResponse struct {
	MessageID             string    `json:"message_id"`
	ChatID                string    `json:"chat_id"`
	RecipientID           string    `json:"recipient_id"`
	Content               string    `json:"content"`
	MessageType           string    `json:"message_type"`
	DeliveredToRecipient  bool      `json:"delivered_to_recipient"`
	SyncedToSenderPrimary bool      `json:"synced_to_sender_primary"`
	CreatedAt             time.Time `json:"created_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	IsFromMe              bool      `json:"is_from_me"`
	FileID                *string   `json:"file_id"`
	FileName              *string   `json:"file_name"`
	FileSize              *int64    `json:"file_size"`
	FileMimeType          *string   `json:"file_mime_type"`
	ViewURL               string    `json:"view_url,omitempty"`
	DownloadURL           string    `json:"download_url,omitempty"`
}

type MessagingEligibilityResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type CreateChatRequest struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type CheckEligibilityRequest struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type SendMessageRequest struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
	Content     string `json:"content" validate:"required,max=5000"`
	MessageType string `json:"message_type" validate:"required,oneof=text image video audio file"`
}

type AcknowledgeDeliveryRequest struct {
	MessageID      string `json:"message_id" validate:"required,uuid"`
	AcknowledgedBy string `json:"acknowledged_by" validate:"required,oneof=recipient sender"`
	Success        bool   `json:"success"`
}

type GetMessagesQuery struct {
	ChatID string `query:"chat_id" validate:"required,uuid"`
	Limit  int32  `query:"limit"`
	Offset int32  `query:"offset"`
}

// Payload structs for handler-service communication
type CheckEligibilityPayload struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type CreateChatPayload struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
}

type SendMessagePayload struct {
	RecipientID string `json:"recipient_id" validate:"required,uuid"`
	Content     string `json:"content" validate:"required,max=5000"`
	MessageType string `json:"message_type" validate:"required,oneof=text image video audio file"`
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

type SyncActionResponse struct {
	ID                 string      `json:"id"`
	UserID             string      `json:"user_id"`
	ActionType         string      `json:"action_type"`
	Payload            interface{} `json:"payload"`
	DeliveredToPrimary bool        `json:"delivered_to_primary"`
	CreatedAt          time.Time   `json:"created_at"`
}

type GetSyncActionsResponse struct {
	Actions []SyncActionResponse `json:"actions"`
	Count   int                  `json:"count"`
}

// Response structs for wrapper objects
type GetMessagesResponse struct {
	Messages            []MessageResponse `json:"messages"`
	Count               int               `json:"count"`
	OtherUserLastReadAt time.Time         `json:"other_user_last_read_at"`
}

type GetUserChatsResponse struct {
	Chats []ChatResponse `json:"chats"`
	Count int            `json:"count"`
}

type AcknowledgeDeliveryResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

type GetFileURLResponse struct {
	ViewURL     string `json:"view_url,omitempty"`
	DownloadURL string `json:"download_url"`
}

type UploadFileResponse struct {
	MessageID    string    `json:"message_id"`
	FileID       string    `json:"file_id"`
	MessageType  string    `json:"message_type"`
	FileMimeType *string   `json:"file_mime_type"`
	ViewURL      string    `json:"view_url,omitempty"`
	DownloadURL  string    `json:"download_url"`
	FileName     *string   `json:"file_name"`
	FileSize     *int64    `json:"file_size"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
