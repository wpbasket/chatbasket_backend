package publicservice

import (
	"chatbasket-api/services"
)

// Service wraps the shared GlobalService for public endpoints
// so both Public and Personal modes can reuse core logic.
type Service struct {
	*services.GlobalService
	AuthSecret []byte
}

// New constructs a public Service from the shared GlobalService.
func New(gs *services.GlobalService, authSecret []byte) *Service {
	return &Service{GlobalService: gs, AuthSecret: authSecret}
}
