package personalhandler

import (
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"chatbasket-api/personal/personalservice"
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

	resp, apiErr := h.service.SendMessageHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
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

	resp, apiErr := h.service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
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
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) UploadFileForMessage(c echo.Context) error {
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

	resp, apiErr := h.service.UploadFileForMessageHandler(c.Request().Context(), c, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
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
