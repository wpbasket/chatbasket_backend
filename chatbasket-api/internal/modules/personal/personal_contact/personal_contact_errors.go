package personal_contact

import (
	"chatbasket-api/internal/platform/kit"
)

// Module-specific error definitions
var (
	// General / Auth Errors
	ErrInvalidUserContext = kit.NewError(401, "unauthorized", "User id is missing or invalid")
	ErrInvalidPayload     = kit.NewError(400, "bad_request", "invalid request payload")

	// Contact Specific Errors
	ErrSelfAddition          = kit.NewError(409, "conflict", "self_addition")
	ErrSelfAdminBlocked      = kit.NewError(403, "forbidden", "self_admin_blocked")
	ErrUserNotFound          = kit.NewError(404, "not_found", "user_not_found")
	ErrUserAdminBlocked      = kit.NewError(403, "forbidden", "user_admin_blocked")
	ErrYouBlockedUser        = kit.NewError(403, "forbidden", "you_blocked_user")
	ErrUserBlockedYou        = kit.NewError(403, "forbidden", "user_blocked_you")
	ErrInvalidNicknameLength = kit.NewError(400, "bad_request", "invalid_nickname_length")
	ErrUserPrivateProfile    = kit.NewError(403, "forbidden", "user_private_profile")

	// Request Errors
	ErrSelfActionNotAllowed    = kit.NewError(409, "conflict", "self_action_not_allowed")
	ErrPendingRequestNotFound  = kit.NewError(404, "not_found", "pending_request_not_found")
	ErrRequestAlreadyProcessed = kit.NewError(409, "conflict", "request_already_processed")
	ErrContactNotFound         = kit.NewError(404, "not_found", "contact_not_found")

	// Block Errors
	ErrSelfBlockNotAllowed = kit.NewError(409, "conflict", "self_block_not_allowed")

	// Field Required Errors
	ErrContactUsernameRequired = kit.NewError(400, "bad_request", "contact_username is required")
	ErrContactUserIdRequired   = kit.NewError(400, "bad_request", "contact_user_id is required")
)

