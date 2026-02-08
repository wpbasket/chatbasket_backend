package personalmodel

import (
	"time"
)

type ChatResponse struct {
	ChatID            string    `json:"chat_id"`
	OtherUserID       string    `json:"other_user_id"`
	OtherUserName     string    `json:"other_user_name"`
	OtherUserUsername string    `json:"other_user_username"`
	AvatarURL         *string   `json:"avatar_url"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type MessageResponse struct {
	MessageID   string    `json:"message_id"`
	ChatID      string    `json:"chat_id"`
	SenderID    string    `json:"sender_id"`
	RecipientID string    `json:"recipient_id"`
	Content     string    `json:"content"`
	MessageType string    `json:"message_type"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
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

type GetFileURLPayload struct {
	MessageID string `json:"message_id" validate:"required,uuid"`
}

// Response structs for wrapper objects
type GetMessagesResponse struct {
	Messages []MessageResponse `json:"messages"`
	Count    int               `json:"count"`
}

type GetUserChatsResponse struct {
	Chats []ChatResponse `json:"chats"`
	Count int            `json:"count"`
}

type AcknowledgeDeliveryResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

type GetFileURLResponse struct {
	FileURL string `json:"file_url"`
}

type UploadFileResponse struct {
	MessageID string    `json:"message_id"`
	FileURL   string    `json:"file_url"`
	FileName  *string   `json:"file_name"`
	FileSize  *int64    `json:"file_size"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
