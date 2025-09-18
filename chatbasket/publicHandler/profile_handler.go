package publicHandler

import (
	"chatbasket/model"
	"chatbasket/publicServices"
	"net/http"

	"github.com/labstack/echo/v4"
	// "github.com/go-playground/validator/v10"
)

// var validate = validator.New()

type ProfileHandler struct {
	Service *publicServices.Service
}

func NewProfileHandler(service *publicServices.Service) *ProfileHandler {
	return &ProfileHandler{Service: service}
}

func (h *ProfileHandler) Logout(c echo.Context) error {
	var payload model.LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid logout payload")
	}
	userId := c.Get("userId").(string)
	sessionId := c.Get("sessionId").(string)

	user, err := h.Service.Logout(c.Request().Context(), &payload, userId, sessionId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	platform := c.Get("platform").(string)
	if platform == "web" {
		// remove these cookies c.SetCookie(sessionCookie) c.SetCookie(userCookie)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			Domain:   "chatbasket.me", // Use this - same as when you set the cookie
			// Domain:   "localhost:8081", // Use this - same as when you set the cookie
			SameSite: http.SameSiteNoneMode,
			MaxAge:   -1,
		}

		userCookie := &http.Cookie{
			Name:     "userId",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			Domain:   "chatbasket.me", // Use this - same as when you set the cookie
			// Domain:   "localhost:8081", // Use this - same as when you set the cookie
			SameSite: http.SameSiteNoneMode,
			MaxAge:   -1,
		}

		c.SetCookie(sessionCookie)
		c.SetCookie(userCookie)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *ProfileHandler) CheckIfUserNameAvailable(c echo.Context) error {
	var payload model.CheckIfUserNameAvailablePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid username payload")
	}
	res, err := h.Service.CheckIfUserNameAvailable(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, res)
}

func (h *ProfileHandler) CreateUserProfile(c echo.Context) error {
	var payload model.CreateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid create user profile payload")
	}

	// if err := validate.Struct(payload); err != nil {
	//     return echo.NewHTTPError(http.StatusBadRequest, "Validation failed: "+err.Error())
	// }

	userId := c.Get("userId").(string)

	user, err := h.Service.CreateUserProfile(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)

}

func (h *ProfileHandler) GetProfile(c echo.Context) error {
	userId := c.Get("userId").(string)
	if userId == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "User id is missing")
	}

	user, err := h.Service.GetProfile(c.Request().Context(), userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *ProfileHandler) UploadProfilePicture(c echo.Context) error {
    err := c.Request().ParseMultipartForm(5 << 20) // 5MB
    if err != nil {
        return c.JSON(http.StatusBadRequest, map[string]string{
            "error": "Failed to parse multipart form",
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
        
        return c.JSON(http.StatusBadRequest, map[string]interface{}{
            "error": "Avatar file not found in request",
            "details": err.Error(),
            "available_file_fields": availableFields,
        })
    }
    
    if fh.Size > 5<<20 {
        return c.JSON(http.StatusBadRequest, "File size exceeds the limit")
    }

    userId := c.Get("userId").(string)
    user, serviceErr := h.Service.UploadUserProfilePicture(c.Request().Context(), fh, userId)

    if serviceErr != nil {
        return c.JSON(serviceErr.Code, serviceErr)
    }

    return c.JSON(http.StatusOK, user)
}

func (h *ProfileHandler) RemoveProfilePicture(c echo.Context) error {
	userId := c.Get("userId").(string)

	user, err := h.Service.RemoveUserProfilePicture(c.Request().Context(), userId)
	if err != nil {	
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)
}



func (h *ProfileHandler) UpdateProfile(c echo.Context) error {
	var payload model.UpdateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid update profile payload")
	}
	userId := c.Get("userId").(string)

	user, err := h.Service.UpdateUserProfile(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)

}
