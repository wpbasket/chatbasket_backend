package personal_profile

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"golang.org/x/crypto/chacha20poly1305"
)

// Character sets for username generation
const (
	letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits  = "0123456789"
)

func generateRandomUsername() (string, error) {
	username := make([]byte, 10)

	// first 4 letters
	for i := 0; i < 4; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		username[i] = letters[idx.Int64()]
	}

	// next 6 digits
	for i := 4; i < 10; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		username[i] = digits[idx.Int64()]
	}

	return string(username), nil
}

func EncryptUsername(username string, encryptionKey []byte, userIDStr string) (string, error) {
	u, err := uuid.Parse(userIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid UUID string: %w", err)
	}

	if len(encryptionKey) != chacha20poly1305.KeySize {
		return "", fmt.Errorf("encryptionKey must be %d bytes, got %d", chacha20poly1305.KeySize, len(encryptionKey))
	}

	aead, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		return "", err
	}

	// Use first 12 bytes of UUID as nonce
	nonce := u[:chacha20poly1305.NonceSize]

	ciphertext := aead.Seal(nonce, nonce, []byte(username), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptUsername(encryptedB64 string, encryptionKey []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	if len(encryptionKey) != chacha20poly1305.KeySize {
		return "", fmt.Errorf("encryptionKey must be %d bytes, got %d", chacha20poly1305.KeySize, len(encryptionKey))
	}

	aead, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		return "", err
	}

	nonceSize := chacha20poly1305.NonceSize
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes, need at least %d", len(raw), nonceSize)
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}
	return string(plaintext), nil
}

// ShouldExposeAvatar determines if a user's avatar should be visible based on global and user-level privacy restrictions.
// Ported from contact_service.go for full fidelity and centralized privacy logic.
func ShouldExposeAvatar(globalRestrictProfile, exceptionGlobalProfile, globalRestrictAvatar, exceptionGlobalAvatar, userRestrictProfile, userRestrictAvatar bool) bool {
	if globalRestrictProfile {
		return exceptionGlobalProfile
	}
	if globalRestrictAvatar {
		return exceptionGlobalAvatar
	}
	if userRestrictProfile {
		return false
	}
	if userRestrictAvatar {
		return false
	}
	return true
}
