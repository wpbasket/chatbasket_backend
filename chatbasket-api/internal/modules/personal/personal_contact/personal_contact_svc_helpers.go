package personal_contact

import (
	"chatbasket-api/internal/modules/personal/personal_contact/internal/personal_contact_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	rpc_personal_contactv1 "chatbasket-api/gen/proto/personal/personal_contact"

	"github.com/google/uuid"
	"golang.org/x/crypto/chacha20poly1305"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (ps *contactService) EncryptNickname(nickname string, ownerID uuid.UUID, contactID uuid.UUID) (string, error) {
	if len(ps.PersonalContactKey) != chacha20poly1305.KeySize {
		return "", fmt.Errorf("PersonalContactKey must be %d bytes", chacha20poly1305.KeySize)
	}

	aead, err := chacha20poly1305.New(ps.PersonalContactKey)
	if err != nil {
		return "", err
	}

	// Use first 12 bytes of ContactUserID as nonce
	nonce := contactID[:chacha20poly1305.NonceSize]

	// Use OwnerUserID as AAD (Additional Authenticated Data) for binding
	aad := ownerID[:]

	ciphertext := aead.Seal(nonce, nonce, []byte(nickname), aad)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (ps *contactService) DecryptNickname(encryptedB64 *string, ownerID uuid.UUID, contactID uuid.UUID) (*string, error) {
	if encryptedB64 == nil || *encryptedB64 == "" {
		return nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(*encryptedB64)
	if err != nil {
		return nil, fmt.Errorf("nickname base64 decode failed: %w", err)
	}

	if len(ps.PersonalContactKey) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("PersonalContactKey must be %d bytes", chacha20poly1305.KeySize)
	}

	aead, err := chacha20poly1305.New(ps.PersonalContactKey)
	if err != nil {
		return nil, err
	}

	nonceSize := chacha20poly1305.NonceSize
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("nickname ciphertext too short")
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]
	aad := ownerID[:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("nickname decryption failed: %w", err)
	}

	res := string(plaintext)
	return &res, nil
}

func (ps *contactService) buildSingleContactForOwner(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (*rpc_personal_contactv1.Contact, error) {
	row, err := ps.PostgresQueries.GetSingleUserContactLite(ctx, personal_contact_store.GetSingleUserContactLiteParams{
		OwnerUserID:   ownerID,
		ContactUserID: contactID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	profilesByID, err := ps.personalProfilePersonalContactProvider.GetContactableProfilesForViewer(ctx, ownerID, []uuid.UUID{contactID})
	if err != nil {
		return nil, err
	}

	profile, ok := profilesByID[contactID]
	if !ok {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "contact profile not found")
	}

	isMutual, err := ps.PostgresQueries.IsAlreadyContact(ctx, personal_contact_store.IsAlreadyContactParams{
		OwnerUserID:   contactID,
		ContactUserID: ownerID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	var nickname *string
	if row.Nickname != nil {
		decrypted, err := ps.DecryptNickname(row.Nickname, ownerID, contactID)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt contact nickname")
		}
		nickname = decrypted
	}

	return &rpc_personal_contactv1.Contact{
		Id:           row.ID.String(),
		Name:         profile.Name,
		Username:     profile.Username,
		Bio:          profile.Bio,
		Nickname:     nickname,
		CreatedAt:    timestamppb.New(row.ContactCreatedAt),
		UpdatedAt:    timestamppb.New(row.ContactUpdatedAt),
		AvatarUrl:    profile.AvatarURL,
		AvatarFileId: profile.AvatarFileId,
		IsMutual:     isMutual,
		ProfileType:  profile.ProfileType,
	}, nil
}
