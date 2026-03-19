package profilekit

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"golang.org/x/crypto/chacha20poly1305"
)

// ----------------------------
// Character sets
// ----------------------------
const (
	letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits  = "0123456789"
)

// ----------------------------
// GenerateRandomUsername
// ----------------------------
// Pattern: 4 Uppercase letters(A-Z) + 6 digits(0-9)
func GenerateRandomUsername() (string, error) {
	username := make([]byte, 10)

	// first 4 letters
	for i := range 4 {
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

// ----------------------------
// Username Validation
// ----------------------------
// Checks if a username matches the pattern
// func ValidateUsername(username string) error {
//  pattern := `^[A-Z]{4}[0-9]{6}$`
//  match, err := regexp.MatchString(pattern, username)
//  if err != nil {
//   return err
//  }
//  if !match {
//   return errors.New("invalid username format: must be 4 uppercase letters + 6 digits")
//  }
//  return nil
// }

// ----------------------------
// Custom validator registration
// ----------------------------
// func RegisterCreateUserProfileValidators(v *validator.Validate) error {
//  return v.RegisterValidation("username", func(fl validator.FieldLevel) bool {
//   username := fl.Field().String()
//   err := ValidateUsername(username)
//   return err == nil
//  })
// }

//
// ---------- ChaCha20-Poly1305 ----------
//

// EncryptUsername encrypts username using ChaCha20-Poly1305 with a UUID string as nonce source. It can be reversed to retrieve the original username.
// It is used for storing the username in encrypted form in the database.
func EncryptUsername(username string, encryptionKey []byte, userIDStr string) (string, error) {
	// Parse UUID string from Appwrite
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

// DecryptUsername decrypts a base64 ciphertext using ChaCha20-Poly1305.
// The nonce is embedded in the ciphertext itself.
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
