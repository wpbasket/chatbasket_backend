package personalHandler

import (
	"chatbasket/model"
	"chatbasket/personalServices"
	"net/http"

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
	var payload model.LogoutPayload
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

func (h *ProfileHandler) CheckIfUserNameAvailable(c echo.Context) error {
	var payload model.CheckIfUserNameAvailablePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid username payload")
	}
	res, apiErr := h.Service.CheckIfUserNameAvailable(c.Request().Context(), &payload)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) CreateUserProfile(c echo.Context) error {
	var payload model.CreateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid create user profile payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.CreateUserProfile(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) GetProfile(c echo.Context) error {
	userId := c.Get("userId").(string)
	if userId == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "User id is missing")
	}
	res, apiErr := h.Service.GetProfile(c.Request().Context(), userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) UploadProfilePicture(c echo.Context) error {
	if err := c.Request().ParseMultipartForm(5 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "Failed to parse multipart form",
			"details": err.Error(),
		})
	}
	if c.Request().MultipartForm == nil {
		return c.JSON(http.StatusBadRequest, "MultipartForm is nil")
	}
	fh, err := c.FormFile("avatar")
	if err != nil {
		availableFields := []string{}
		if c.Request().MultipartForm != nil && c.Request().MultipartForm.File != nil {
			for field := range c.Request().MultipartForm.File {
				availableFields = append(availableFields, field)
			}
		}
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":                 "Avatar file not found in request",
			"details":               err.Error(),
			"available_file_fields": availableFields,
		})
	}
	if fh.Size > 5<<20 {
		return c.JSON(http.StatusBadRequest, "File size exceeds the limit")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.UploadUserProfilePicture(c.Request().Context(), fh, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) RemoveProfilePicture(c echo.Context) error {
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.RemoveUserProfilePicture(c.Request().Context(), userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) UpdateProfile(c echo.Context) error {
	var payload model.UpdateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid update profile payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.UpdateUserProfile(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}
