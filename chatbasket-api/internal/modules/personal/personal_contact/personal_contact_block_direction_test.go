package personal_contact

import (
	"context"
	"errors"
	"testing"

	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Privacy: a blocked pair must produce an identical outward error no matter
// which side created the block. Directional codes let any user probe whether
// a stranger blocked them.
func TestCreateContact_BlockedEitherDirection_IdenticalGenericError(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()

	call := func(blockStatus int32) error {
		profile := &mockContactProfileProvider{
			coreProfile:     &personal_profile.UserCoreProfile{ID: contactID, ProfileType: "public"},
			isEitherBlocked: blockStatus,
		}
		service, _ := newMockContactService(t, profile)
		_, err := service.CreateContact(context.Background(), &CreateContactPayload{
			ContactUserId: contactID.String(),
		}, kit.UserId{UuidUserId: ownerID})
		return err
	}

	errBlockedByMe := call(1)
	errBlockedByThem := call(2)
	require.Error(t, errBlockedByMe)
	require.Error(t, errBlockedByThem)

	msgByMe, ok := errBlockedByMe.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errBlockedByMe)
	msgByThem, ok := errBlockedByThem.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errBlockedByThem)

	assert.Equal(t, msgByMe.Error(), msgByThem.Error(), "block direction must not leak via error code")
	assert.Equal(t, "action_not_permitted", msgByMe.Error())
}

// Privacy: checkBlockStatus gates 6 contact operations. When the pair is
// user-blocked in either direction, the wire error must carry no direction
// flags — otherwise every accept/reject/undo/nickname call leaks it.
func TestCheckBlockStatus_UserBlockEitherDirection_NoDirectionFlags(t *testing.T) {
	ownerID := uuid.New()
	targetID := uuid.New()

	call := func(status *personal_profile.BlockStatusResult) error {
		profile := &mockContactProfileProvider{blockStatus: status}
		service, _ := newMockContactService(t, profile)
		_, err := service.UndoContactRequest(context.Background(), &UndoContactRequestPayload{
			ContactUserId: targetID.String(),
		}, kit.UserId{UuidUserId: ownerID})
		return err
	}

	errByMe := call(&personal_profile.BlockStatusResult{
		IsBlocked:                      true,
		IsTargetUserBlockedByRequester: true,
	})
	errByThem := call(&personal_profile.BlockStatusResult{
		IsBlocked:                      true,
		IsRequesterUserBlockedByTarget: true,
	})
	require.Error(t, errByMe)
	require.Error(t, errByThem)

	peByMe, ok := errByMe.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errByMe)
	peByThem, ok := errByThem.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errByThem)
	assert.Equal(t, peByMe.Error(), peByThem.Error(), "block direction must not leak via error code")

	for _, err := range []error{errByMe, errByThem} {
		var detailed kit.DetailedProcessedError
		if errors.As(err, &detailed) {
			assert.Nil(t, detailed.Details(), "user-block error must carry no details at all")
		}
	}
}
