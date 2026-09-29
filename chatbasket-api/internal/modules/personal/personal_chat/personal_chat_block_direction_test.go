package personal_chat

import (
	"context"
	"testing"

	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Privacy: messaging eligibility must not reveal which side created the
// block. Directional reasons let any user probe whether a stranger blocked
// them via tap-to-message, create-chat, send, presign, or confirm.
func TestCheckMessagingEligibility_BlockedEitherDirection_IdenticalReason(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	call := func(blockStatus int32) (string, *string, int32, error) {
		svc := &chatService{
			AuthProvider:    &mockAuthProvider{},
			ProfileProvider: &mockProfileProvider{},
			ContactProvider: &mockContactProvider{
				getMessagingBlockStatusFunc: func(_ context.Context, _, _ uuid.UUID) (int32, error) {
					return blockStatus, nil
				},
			},
		}
		return svc.CheckMessagingEligibility(context.Background(), senderID, recipientID)
	}

	reasonByMe, _, _, err := call(1)
	require.NoError(t, err)
	reasonByThem, _, _, err := call(2)
	require.NoError(t, err)

	assert.Equal(t, reasonByMe, reasonByThem, "block direction must not leak via eligibility reason")
	assert.Equal(t, "user_blocked", reasonByMe)

	errByMe := messagingEligibilityError(reasonByMe)
	errByThem := messagingEligibilityError(reasonByThem)
	peByMe, ok := errByMe.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errByMe)
	peByThem, ok := errByThem.(kit.ProcessedError)
	require.True(t, ok, "expected ProcessedError, got %T", errByThem)
	assert.Equal(t, peByMe.Kind(), peByThem.Kind(), "block direction must not leak via error type")
	assert.Equal(t, "messaging_not_allowed_user_blocked", peByMe.Kind())
	assert.Equal(t, peByMe.Error(), peByThem.Error())
}
