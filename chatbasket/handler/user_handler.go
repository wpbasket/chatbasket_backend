package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"chatbasket/model"
	"chatbasket/services"
)

type UserHandler struct {
	Service *services.UserService
}

func NewUserHandler(service *services.UserService) *UserHandler {
	return &UserHandler{Service: service}
}

func (h *UserHandler) Signup(c echo.Context) error {
	var payload model.SignupPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid signup payload: "+err.Error())
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	// Create user via service
	user, err := h.Service.Signup(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Return sanitized user info
	return c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) AcountVerification(c echo.Context) error {
	var payload model.AccountVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid OTP payload")
	}

	if payload.Email == "" || payload.Name == "" || payload.Secret == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	session, err := h.Service.AccountVerification(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "OTP verified, session created",
		"session": session,
	})
}


func (h *UserHandler) Login(c echo.Context) error {
	var payload model.LoginPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid login payload: "+err.Error())
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	// Login via service
	user, err := h.Service.Login(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Return sanitized user info
	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) LoginVerification(c echo.Context) error {
	var payload model.LoginVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid OTP payload")
	}

	if payload.Email == "" || payload.Secret == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	user, err := h.Service.LoginVerification(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "OTP verified, session created",
		"user": user,
	})
}