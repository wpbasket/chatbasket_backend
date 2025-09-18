package publicServices

import (
	"chatbasket/services"
)

// Service wraps the shared GlobalService for public endpoints
// so both Public and Personal modes can reuse core logic.
type Service struct {
	*services.GlobalService
}

// New constructs a public Service from the shared GlobalService.
func New(gs *services.GlobalService) *Service {
	return &Service{GlobalService: gs}
}
