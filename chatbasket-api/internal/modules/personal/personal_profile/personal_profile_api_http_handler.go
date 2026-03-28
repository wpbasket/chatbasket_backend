package personal_profile

import (
	"net/http"
	"strings"

	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type profileHandler struct {
	Service *profileService
}

func newProfileHandler(service *profileService) *profileHandler {
	return &profileHandler{Service: service}
}

func (h *profileHandler) CreateUserProfile(c *echo.Context) error {
	var payload createUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid create user profile payload")
	}
	stringUserId, ok := c.Get("userId").(string)
	if !ok || stringUserId == "" {
		return ErrInvalidUserContext
	}
	email, ok := c.Get("email").(string)
	if !ok || email == "" {
		return ErrInvalidEmailContext
	}
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return ErrInvalidUserContext
	}

	res, err := h.Service.CreateUserProfile(c.Request().Context(), &payload, &kit.UserId{StringUserId: stringUserId, UuidUserId: uuidUserId}, email)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) GetProfile(c *echo.Context) error {
	stringUserId, ok := c.Get("userId").(string)
	if !ok || stringUserId == "" {
		return ErrInvalidUserContext
	}
	email, ok := c.Get("email").(string)
	if !ok || email == "" {
		return ErrInvalidEmailContext
	}
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return ErrInvalidUserContext
	}
	userID := kit.UserId{StringUserId: stringUserId, UuidUserId: uuidUserId}
	res, err := h.Service.GetProfile(c.Request().Context(), &userID, email)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) UploadProfilePicture(c *echo.Context) error {
	err := c.Request().ParseMultipartForm(5 << 20) // 5MB
	if err != nil {
		return kit.NewError(400, "bad_request", "Failed to parse multipart form: "+err.Error())
	}

	if c.Request().MultipartForm == nil {
		return ErrMultipartFormMissing
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

		return kit.NewError(400, "bad_request", message)
	}

	if fh.Size > 5<<20 {
		return ErrFileSizeExceeded
	}

	userId, ok := c.Get("userId").(string)
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !ok || !okUUID {
		return ErrInvalidUserContext
	}
	user, err := h.Service.UploadUserProfilePicture(c.Request().Context(), fh, kit.UserId{StringUserId: userId, UuidUserId: uuidUserId})

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (h *profileHandler) RemoveProfilePicture(c *echo.Context) error {
	userId, ok := c.Get("userId").(string)
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !ok || !okUUID {
		return ErrInvalidUserContext
	}

	res, err := h.Service.RemoveUserProfilePicture(c.Request().Context(), kit.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) UpdateProfile(c *echo.Context) error {
	var payload updateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid update profile payload: "+err.Error())
	}
	userId, ok := c.Get("userId").(string)
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !ok || !okUUID {
		return ErrInvalidUserContext
	}

	res, err := h.Service.UpdateUserProfile(c.Request().Context(), &payload, kit.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

