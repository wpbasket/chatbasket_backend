package handler

import (
	"chatbasket/model"
	"chatbasket/services"
	"net/http"
	"github.com/labstack/echo/v4"
)

type ProfileHandler struct{
	Service *services.GlobalService
}

func NewProfileHandler(service *services.GlobalService) *ProfileHandler {
	return &ProfileHandler{Service: service}
}


func (h *ProfileHandler) Logout(c echo.Context) error {
	var payload model.LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid logout payload")
	}
	userId:= c.Get("userId").(string)
	sessionId := c.Get("sessionId").(string)
	
	user, err := h.Service.Logout(c.Request().Context(), &payload, userId, sessionId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	platform:= c.Get("platform").(string)
	if platform == "web" {
		// remove these cookies c.SetCookie(sessionCookie) c.SetCookie(userCookie)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1, // Delete the cookie
		}

		userCookie := &http.Cookie{
			Name:  "userId",
			Value: "",
			Path:  "/",
			HttpOnly: false,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1, // Delete the cookie
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
	res,err := h.Service.CheckIfUserNameAvailable(c.Request().Context(), &payload)
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
	userId:= c.Get("userId").(string)
	
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

func (h *ProfileHandler) UpdateProfile(c echo.Context) error {
	var payload model.CreateUserProfilePayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid update profile payload")
	}
	userId:= c.Get("userId").(string)
	
	user, err := h.Service.UpdateUserProfile(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}
	
	return c.JSON(http.StatusOK, user)
	
}
