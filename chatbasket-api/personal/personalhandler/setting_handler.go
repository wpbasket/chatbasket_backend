package personalhandler

import (
	// "chatbasket/model"
	"chatbasket/personal/personalservice"
	// "net/http"
	// "github.com/labstack/echo/v4"
)

// SettingHandler handles personal-mode settings endpoints
// It uses personalservice.Service which wraps the shared services.GlobalService
// and is intended for personal mode specific behaviors.
type SettingHandler struct {
	Service *personalservice.Service
}

func NewSettingHandler(service *personalservice.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}
