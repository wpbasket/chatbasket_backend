package personal_contact

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/chacha20poly1305"
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
