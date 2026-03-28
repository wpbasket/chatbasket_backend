package personal_profile

import (
	"chatbasket-api/internal/platform/kit"
)

// Module-specific error definitions
var (
	ErrInvalidUserContext    = kit.NewError(500, "internal_server_error", "Invalid user context")
	ErrInvalidEmailContext   = kit.NewError(500, "internal_server_error", "Invalid email context")
	ErrInvalidPayload        = kit.NewError(400, "bad_request", "Invalid request payload")
	ErrFailedParseForm       = kit.NewError(400, "bad_request", "Failed to parse multipart form")
	ErrMultipartFormMissing  = kit.NewError(400, "bad_request", "Multipart form is missing")
	ErrAvatarNotFound        = kit.NewError(400, "bad_request", "Avatar file not found in request")
	ErrFileSizeExceeded      = kit.NewError(400, "bad_request", "File size exceeds the 5MB limit")
)

