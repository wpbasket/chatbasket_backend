package personalHandler

import (
	// "chatbasket/model"
	"chatbasket/personalServices"
	// "net/http"

	// "github.com/labstack/echo/v4"
)

// SettingHandler handles personal-mode settings endpoints
// It uses personalServices.Service which wraps the shared services.GlobalService
// and is intended for personal mode specific behaviors.
type SettingHandler struct {
	Service *personalServices.Service
}

func NewSettingHandler(service *personalServices.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}

