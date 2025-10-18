package personalHandler

import (
	"chatbasket/model"
	"chatbasket/personalModel"
	"chatbasket/personalServices"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ProfileHandler handles personal-mode profile endpoints
// It uses personalServices.Service which wraps the shared services.GlobalService
// and is intended for personal mode specific behaviors.
type ProfileHandler struct {
	Service *personalServices.Service
}

func NewProfileHandler(service *personalServices.Service) *ProfileHandler {
	return &ProfileHandler{Service: service}
}

func (h *ProfileHandler) Logout(c echo.Context) error {
	var payload personalmodel.LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid logout payload")
	}
	userId := c.Get("userId").(string)
	sessionId := c.Get("sessionId").(string)

	res, apiErr := h.Service.Logout(c.Request().Context(), &payload, userId, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}



func (h *ProfileHandler) CreateUserProfile(c echo.Context) error {
	var payload personalmodel.CreateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid create user profile payload")
	}
	stringUserId := c.Get("userId").(string)
	email := c.Get("email").(string)
	uuidUserId := c.Get("uuidUserId").(uuid.UUID)

	res, apiErr := h.Service.CreateUserProfile(c.Request().Context(), &payload, &model.UserId{StringUserId: stringUserId, UuidUserId: uuidUserId},email)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) GetProfile(c echo.Context) error {
	stringUserId := c.Get("userId").(string)
	email := c.Get("email").(string)
	uuidUserId := c.Get("uuidUserId").(uuid.UUID)
	res, apiErr := h.Service.GetProfile(c.Request().Context(), model.UserId{StringUserId: stringUserId, UuidUserId: uuidUserId},email)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}




func (h *ProfileHandler) UploadProfilePicture(c echo.Context) error {
	err := c.Request().ParseMultipartForm(5 << 20) // 5MB
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Failed to parse multipart form: " + err.Error(),
			Type:    "bad_request",
		})
	}

	if c.Request().MultipartForm == nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Multipart form is missing",
			Type:    "bad_request",
		})
	}

	fh, err := c.FormFile("avatar")
	if err != nil {
		availableFields := []string{}
		if c.Request().MultipartForm != nil && c.Request().MultipartForm.File != nil {
			for field := range c.Request().MultipartForm.File {
				availableFields = append(availableFields, field)
			}
		}

		message := "Avatar file not found in request: " + err.Error()
		if len(availableFields) > 0 {
			message += ". Available file fields: " + strings.Join(availableFields, ", ")
		}

		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: message,
			Type:    "bad_request",
		})
	}

	if fh.Size > 5<<20 {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "File size exceeds the 5MB limit",
			Type:    "bad_request",
		})
	}

	userId, ok := c.Get("userId").(string)
	uuidUserId := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	user, serviceErr := h.Service.UploadUserProfilePicture(c.Request().Context(), fh, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})

	if serviceErr != nil {
		return c.JSON(serviceErr.Code, serviceErr)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *ProfileHandler) RemoveProfilePicture(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	uuidUserId := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	user, err := h.Service.RemoveUserProfilePicture(c.Request().Context(), model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *ProfileHandler) UpdateProfile(c echo.Context) error {
	var payload personalmodel.UpdateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid update profile payload: " + err.Error(),
			Type:    "bad_request",
		})
	}
	userId, ok := c.Get("userId").(string)
	uuidUserId := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	user, err := h.Service.UpdateUserProfile(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)

}