package personalServices

import (
	"chatbasket/services"
)

// Service wraps the shared GlobalService for personal-mode endpoints.
// Extend with personal-specific utilities as the feature evolves.
type Service struct {
	*services.GlobalService
}

// New constructs a personal Service from the shared GlobalService.
func New(gs *services.GlobalService) *Service {
	return &Service{GlobalService: gs}
}
