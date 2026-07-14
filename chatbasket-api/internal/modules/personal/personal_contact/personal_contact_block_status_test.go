package personal_contact

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockStatusProfileProvider struct {
	requesterAdmin               bool
	targetAdmin                  bool
	requesterUserBlockedByTarget bool
	targetUserBlockedByRequester bool
	targetID                     uuid.UUID
	batchCalls                   int
}

func (p *blockStatusProfileProvider) IsUserAdminBlocked(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (p *blockStatusProfileProvider) GetUserCoreProfile(context.Context, uuid.UUID) (*personal_profile.UserCoreProfile, error) {
	return nil, nil
}

func (p *blockStatusProfileProvider) GetContactableProfilesForViewer(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	return nil, nil
}

func (p *blockStatusProfileProvider) FindContactableUserByUsername(context.Context, uuid.UUID, string) (*personal_profile.ContactLookupResult, error) {
	return &personal_profile.ContactLookupResult{ID: p.targetID, Exists: true}, nil
}

func (p *blockStatusProfileProvider) CreateUserBlock(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (p *blockStatusProfileProvider) IsEitherBlocked(context.Context, uuid.UUID, uuid.UUID) (int32, error) {
	return 0, nil
}

func (p *blockStatusProfileProvider) IsBlockedBetweenUsers(context.Context, uuid.UUID, uuid.UUID) (*personal_profile.BlockStatusResult, error) {
	return &personal_profile.BlockStatusResult{
		IsBlocked:                       p.requesterAdmin || p.targetAdmin || p.requesterUserBlockedByTarget || p.targetUserBlockedByRequester,
		IsRequesterAdminBlocked:         p.requesterAdmin,
		IsTargetAdminBlocked:            p.targetAdmin,
		IsRequesterUserBlockedByTarget:  p.requesterUserBlockedByTarget,
		IsTargetUserBlockedByRequester:  p.targetUserBlockedByRequester,
	}, nil
}

func (p *blockStatusProfileProvider) IsBlockedBetweenUsersBatch(_ context.Context, _ uuid.UUID, targetIDs []uuid.UUID) ([]*personal_profile.BlockStatusResult, error) {
	p.batchCalls++
	results := make([]*personal_profile.BlockStatusResult, len(targetIDs))
	for i, id := range targetIDs {
		results[i] = &personal_profile.BlockStatusResult{
			IsBlocked:                       p.requesterAdmin || p.targetAdmin || p.requesterUserBlockedByTarget || p.targetUserBlockedByRequester,
			IsRequesterAdminBlocked:         p.requesterAdmin,
			IsTargetAdminBlocked:            p.targetAdmin,
			IsRequesterUserBlockedByTarget:  p.requesterUserBlockedByTarget,
			IsTargetUserBlockedByRequester:  p.targetUserBlockedByRequester,
			TargetID:                        id,
		}
	}
	return results, nil
}

func TestNewErrorWithDetails_BlockStatus(t *testing.T) {
	flags := personal_profile.BlockStatusFlags{
		IsRequesterAdminBlocked:        true,
		IsTargetAdminBlocked:           true,
		IsRequesterUserBlockedByTarget: true,
		IsTargetUserBlockedByRequester: true,
	}
	err := kit.NewErrorWithDetails(http.StatusForbidden, "forbidden", "blocked", flags)
	require.Error(t, err)

	var processed kit.ProcessedError
	if !errors.As(err, &processed) {
		t.Fatalf("returned error does not implement ProcessedError: %v", err)
	}
	if processed.Status() != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", processed.Status(), http.StatusForbidden)
	}
	if processed.Error() != "blocked" {
		t.Fatalf("message = %q, want %q", processed.Error(), "blocked")
	}

	var detailed kit.DetailedProcessedError
	if !errors.As(err, &detailed) {
		t.Fatalf("returned error does not implement DetailedProcessedError: %v", err)
	}
	got, ok := detailed.Details().(personal_profile.BlockStatusFlags)
	if !ok {
		t.Fatalf("details is not BlockStatusFlags, got %T", detailed.Details())
	}
	if got != flags {
		t.Fatalf("details = %+v, want %+v", got, flags)
	}
}

func TestNewErrorWithDetails_NoDetails(t *testing.T) {
	err := kit.NewErrorWithDetails(http.StatusInternalServerError, "internal_server_error", "boom", nil)
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusInternalServerError, processed.Status())
	assert.Equal(t, "boom", processed.Error())
}

func TestContactEndpointsRejectBlockedPairs(t *testing.T) {
	requesterID := uuid.New()
	targetID := uuid.New()
	provider := &blockStatusProfileProvider{requesterAdmin: true, targetID: targetID}
	service := &contactService{
		personalProfilePersonalContactProvider: provider,
	}
	userID := kit.UserId{UuidUserId: requesterID}

	tests := []struct {
		name           string
		call           func() error
		expectedMessage string
	}{
		{
			name: "check existence",
			call: func() error {
				_, err := service.CheckContactExistance(context.Background(), &CheckContactExistancePayload{ContactUsername: "target"}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
		{
			name: "delete",
			call: func() error {
				_, err := service.DeleteContact(context.Background(), &DeleteContactPayload{ContactUserId: []string{targetID.String()}}, userID)
				return err
			},
			expectedMessage: "self_admin_blocked",
		},
		{
			name: "accept request",
			call: func() error {
				_, err := service.AcceptContactRequest(context.Background(), &AcceptContactRequestPayload{ContactUserId: targetID.String()}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
		{
			name: "reject request",
			call: func() error {
				_, err := service.RejectContactRequest(context.Background(), &RejectContactRequestPayload{ContactUserId: targetID.String()}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
		{
			name: "undo request",
			call: func() error {
				_, err := service.UndoContactRequest(context.Background(), &UndoContactRequestPayload{ContactUserId: targetID.String()}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
		{
			name: "update nickname",
			call: func() error {
				_, err := service.UpdateContactNickname(context.Background(), &UpdateContactNicknamePayload{ContactUserId: targetID.String()}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
		{
			name: "remove nickname",
			call: func() error {
				_, err := service.RemoveContactNickname(context.Background(), &RemoveContactNicknamePayload{ContactUserId: targetID.String()}, userID)
				return err
			},
			expectedMessage: "blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			var processed kit.ProcessedError
			if !errors.As(err, &processed) {
				t.Fatalf("returned a non-processed error: %v", err)
			}
			if processed.Status() != http.StatusForbidden || processed.Error() != tt.expectedMessage {
				t.Fatalf("returned (%d, %q), want (%d, %q)", processed.Status(), processed.Error(), http.StatusForbidden, tt.expectedMessage)
			}
		})
	}

	if provider.batchCalls != 1 {
		t.Fatalf("batch block-status checks = %d, want 1 for DeleteContact", provider.batchCalls)
	}
}
