package personal_profile

import (
	"encoding/base64"
	"net/http"
	"strings"

	"chatbasket-api/internal/platform/kit"

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
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	email, ok := c.Get("email").(string)
	if !ok || email == "" {
		return ErrInvalidEmailContext
	}

	res, err := h.Service.CreateUserProfile(c.Request().Context(), &payload, &userID, email)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) GetProfile(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	email, ok := c.Get("email").(string)
	if !ok || email == "" {
		return ErrInvalidEmailContext
	}
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

	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return ErrInvalidUserContext
	}
	user, err := h.Service.UploadUserProfilePicture(c.Request().Context(), fh, userID)

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (h *profileHandler) RemoveProfilePicture(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return ErrInvalidUserContext
	}

	res, err := h.Service.RemoveUserProfilePicture(c.Request().Context(), userID)
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
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return ErrInvalidUserContext
	}

	res, err := h.Service.UpdateUserProfile(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) UploadE2EEPublicKey(c *echo.Context) error {
	var payload uploadE2EEPublicKeyPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid upload E2EE key payload: "+err.Error())
	}

	if payload.E2eePublicKey == "" {
		return kit.NewError(http.StatusBadRequest, "bad_request", "e2ee_public_key is required")
	}

	if len(payload.E2eePublicKey) != 44 {
		return kit.NewError(http.StatusBadRequest, "bad_request", "e2ee_public_key must be exactly 44 characters (Base64 X25519)")
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.E2eePublicKey)
	if err != nil || len(decoded) != 32 {
		return kit.NewError(http.StatusBadRequest, "bad_request", "e2ee_public_key must be a valid base64-encoded 32-byte X25519 public key")
	}

	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return ErrInvalidUserContext
	}

	res, err := h.Service.SaveE2EEPublicKey(c.Request().Context(), userID, payload.E2eePublicKey)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, res)
}

func (h *profileHandler) GetE2EEPublicKey(c *echo.Context) error {
	var payload getE2EEPublicKeyPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid request: "+err.Error())
	}

	if payload.UserID == "" {
		return kit.NewError(http.StatusBadRequest, "bad_request", "user_id is required")
	}

	uuidVal, err := kit.StringToUUID(payload.UserID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid user_id format")
	}

	pubKey, err := h.Service.GetE2EEPublicKey(c.Request().Context(), uuidVal)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &getE2EEPublicKeyResponse{
		E2eePublicKey: pubKey,
	})
}
