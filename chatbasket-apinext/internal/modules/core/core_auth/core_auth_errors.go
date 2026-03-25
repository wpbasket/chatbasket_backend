package core_auth

import (
	"chatbasket-apinext/internal/platform/kit"
)

// Module-specific error definitions
var (
	ErrInvalidUserContext    = kit.NewError(500, "internal_server_error", "Invalid user context")
	ErrInvalidSessionContext = kit.NewError(500, "internal_server_error", "Invalid session context")
	ErrInvalidPayload        = kit.NewError(400, "bad_request", "Invalid request payload")
	ErrMissingRequired       = kit.NewError(400, "missing_value", "Missing required fields")
	ErrInvalidUpdateType     = kit.NewError(400, "bad_request", "Invalid update type")
	ErrInvalidUpdateID       = kit.NewError(400, "bad_request", "Invalid update ID")
	ErrInternalServerError   = kit.NewError(500, "internal_server_error", "Internal server error")
	ErrInvalidExpiryFormat   = kit.NewError(500, "internal_server_error", "Invalid session expiry format")
)
