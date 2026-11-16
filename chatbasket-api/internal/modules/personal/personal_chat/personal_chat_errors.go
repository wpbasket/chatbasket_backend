package personal_chat

import (
	"fmt"
	"net/http"

	"chatbasket-api/internal/platform/kit"
)

const (
	EligibilityAllowed           = "allowed"
	EligibilityNotInContacts     = "not_in_contacts"
	EligibilityRecipientPrivate  = "recipient_private"
	EligibilityBlocked           = "blocked" // Legacy, keep for safety
	EligibilityUserBlocked       = "user_blocked"
	EligibilityAdminBlocked      = "admin_blocked"
	EligibilityNoPrimaryDevice   = "no_primary_device"
	EligibilityRecipientNotFound = "recipient_not_found"
	EligibilityNoE2EE            = "no_e2ee"
)

// Module-specific error definitions
var (
	ErrTooManyMessageIDs = kit.NewError(http.StatusBadRequest, "invalid_request", fmt.Sprintf("Cannot process more than %d message IDs per request", MaxMessageIDsPerRequest))
)

func messagingEligibilityError(eligibility string) error {
	errType := "messaging_not_allowed"
	switch eligibility {
	case EligibilityNotInContacts:
		errType = "messaging_not_allowed_not_in_contacts"
	case EligibilityRecipientPrivate:
		errType = "messaging_not_allowed_recipient_private"
	case EligibilityBlocked, EligibilityUserBlocked:
		errType = "messaging_not_allowed_user_blocked"
	case EligibilityAdminBlocked:
		errType = "messaging_not_allowed_admin_blocked"
	case EligibilityNoPrimaryDevice:
		errType = "messaging_not_allowed_no_primary_device"
	case EligibilityRecipientNotFound:
		errType = "messaging_not_allowed_recipient_not_found"
	case EligibilityNoE2EE:
		errType = "messaging_not_allowed_no_e2ee"
	}

	return kit.NewError(http.StatusForbidden, errType, eligibility)
}
