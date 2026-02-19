package personalhandler

import (
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"chatbasket-api/personal/personalservice"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ChatHandler struct {
	service *personalservice.Service
}

func (h *ChatHandler) CheckEligibility(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.CheckEligibilityPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.CheckEligibilityHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func NewChatHandler(service *personalservice.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) CreateChat(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.CreateChatPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.CreateChatHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) SendMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.SendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.SendMessageHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, c.Get("isPrimary").(bool))
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetMessages(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetMessagesHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) AcknowledgeDelivery(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.AcknowledgeDeliveryPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	sessionId, _ := c.Get("sessionId").(string)

	resp, apiErr := h.service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetUserChats(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	resp, apiErr := h.service.GetUserChatsHandler(c.Request().Context(), model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		log.Printf("[ChatHandler] GetUserChats failed for user %s: %v", userId, apiErr)
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetPendingMessages(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetPendingMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetPendingMessagesHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) UploadFileForMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	log.Printf("[ChatHandler] UploadFileForMessage received from user: %s", userId)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	resp, apiErr := h.service.UploadFileForMessageHandler(c.Request().Context(), c, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, c.Get("isPrimary").(bool))
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetFileURL(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetFileURLPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetFileURLHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) MarkChatRead(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.MarkChatReadPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.MarkChatReadHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *ChatHandler) UnsendMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.UnsendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		// Default to false if missing (safe fallback) or handle error
		isPrimary = false
	}

	apiErr := h.service.UnsendMessageHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *ChatHandler) DeleteMessageForMe(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.DeleteMessageForMePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.DeleteMessageForMeHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func (h *ChatHandler) GetSyncActions(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetSyncActionsPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetSyncActionsHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		log.Printf("[ChatHandler] GetSyncActions failed for user %s: %v", userId, apiErr)
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) AcknowledgeSyncAction(c echo.Context) error {
	var payload personalmodel.AcknowledgeSyncActionPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.AcknowledgeSyncActionHandler(c.Request().Context(), &payload, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}
