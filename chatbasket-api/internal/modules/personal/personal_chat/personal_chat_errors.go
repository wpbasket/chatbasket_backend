package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"net/http"
)

const (
	EligibilityAllowed            = "allowed"
	EligibilityNotInContacts      = "not_in_contacts"
	EligibilityRecipientPrivate   = "recipient_private"
	EligibilityBlocked            = "blocked" // Legacy, keep for safety
	EligibilityBlockedByRecipient = "blocked_by_recipient"
	EligibilityBlockedByMe        = "blocked_by_me"
	EligibilityAdminBlocked       = "admin_blocked"
	EligibilityNoPrimaryDevice    = "no_primary_device"
	EligibilityRecipientNotFound  = "recipient_not_found"
	EligibilityNoE2EE             = "no_e2ee"
)

func messagingEligibilityError(eligibility string) error {
	errType := "messaging_not_allowed"
	switch eligibility {
	case EligibilityNotInContacts:
		errType = "messaging_not_allowed_not_in_contacts"
	case EligibilityRecipientPrivate:
		errType = "messaging_not_allowed_recipient_private"
	case EligibilityBlocked, EligibilityBlockedByRecipient:
		errType = "messaging_not_allowed_blocked_by_recipient"
	case EligibilityBlockedByMe:
		errType = "messaging_not_allowed_blocked_by_me"
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

