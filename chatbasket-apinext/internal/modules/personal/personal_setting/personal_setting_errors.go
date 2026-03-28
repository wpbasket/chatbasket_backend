package personal_setting

import (
	"chatbasket-apinext/internal/platform/kit"
)

// Module-specific error definitions
var (
	ErrInvalidUserContext    = kit.NewError(401, "unauthorized", "Unauthorized")
	ErrInvalidSessionContext = kit.NewError(401, "unauthorized", "No session context")
	ErrInvalidPayload        = kit.NewError(400, "bad_request", "Invalid request payload")
)
