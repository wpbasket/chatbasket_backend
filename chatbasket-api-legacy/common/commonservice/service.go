package commonservice

import (
	"chatbasket-api/services"
)

// Service wraps the shared GlobalService for common authenticated endpoints
// used by both Public and Personal modes (settings, logout, etc.)
type Service struct {
	*services.GlobalService
	AuthSecret []byte
}

// New constructs a common Service from the shared GlobalService.
func New(gs *services.GlobalService, authSecret []byte) *Service {
	return &Service{GlobalService: gs, AuthSecret: authSecret}
}
