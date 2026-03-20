package personal_setting

import (
	"chatbasket-apinext/internal/platform/kit"
)

// Module-specific error definitions
var (
	ErrInvalidUserContext    = kit.NewError(500, "internal_server_error", "Invalid user context")
	ErrInvalidSessionContext = kit.NewError(500, "internal_server_error", "Invalid session context")
	ErrInvalidPayload        = kit.NewError(400, "bad_request", "Invalid request payload")
)
