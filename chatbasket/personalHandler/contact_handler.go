package personalHandler

import (
	"chatbasket/model"
	"chatbasket/personalModel"
	"chatbasket/personalServices"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"net/http"
)

type ContactHandler struct {
	Service *personalServices.Service
}

func NewContactHandler(service *personalServices.Service) *ContactHandler {
	return &ContactHandler{Service: service}
}

func (h *ContactHandler) GetContacts(c echo.Context) error {
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
	contacts, err := h.Service.GetContacts(c.Request().Context(), model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusOK, contacts)
}

func (h *ContactHandler) CreateContact(c echo.Context) error {
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

	var payload personalmodel.CreateContactPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	res, apiErr := h.Service.CreateContact(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ContactHandler) CheckContactExistance(c echo.Context) error {
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

	var payload personalmodel.CheckContactExistancePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	if payload.ContactUsername == "" {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "contact_username is required",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.Service.CheckContactExistance(
		c.Request().Context(),
		&payload,
		model.UserId{StringUserId: userId, UuidUserId: uuidUserId},
	)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *ContactHandler) AcceptContactRequest(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}
	uid, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}

	var payload personalmodel.AcceptContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"})
	}

	res, apiErr := h.Service.AcceptContactRequest(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uid})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ContactHandler) RejectContactRequest(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}
	uid, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}

	var payload personalmodel.RejectContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"})
	}

	res, apiErr := h.Service.RejectContactRequest(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uid})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ContactHandler) DeleteContact(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}
	uid, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}

	var payload personalmodel.DeleteContactPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"})
	}

	if len(payload.ContactUserId) == 0 {
		return c.JSON(http.StatusBadRequest, &model.ApiError{Code: http.StatusBadRequest, Message: "contact_user_id is required", Type: "bad_request"})
	}

	res, apiErr := h.Service.DeleteContact(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uid})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ContactHandler) UndoContactRequest(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}
	uid, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}

	var payload personalmodel.UndoContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"})
	}

	res, apiErr := h.Service.UndoContactRequest(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uid})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ContactHandler) GetContactRequests(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}
	uid, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{Code: http.StatusUnauthorized, Message: "User id is missing or invalid", Type: "unauthorized"})
	}

	resp, apiErr := h.Service.GetContactRequests(c.Request().Context(), model.UserId{StringUserId: userId, UuidUserId: uid})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}