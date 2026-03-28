package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/chacha20poly1305"
)

//
// ---------- HMAC-SHA256 ----------
//

// ComputeHMAC is a generic helper to compute HMAC-SHA256 hex string.
func ComputeHMAC(data string, secretKey []byte) (string, error) {
	mac := hmac.New(sha256.New, secretKey)
	if _, err := mac.Write([]byte(data)); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyHMAC compares a data string against stored HMAC hex securely.
func VerifyHMAC(data string, storedHex string, secretKey []byte) (bool, error) {
	computedHex, err := ComputeHMAC(data, secretKey)
	if err != nil {
		return false, err
	}

	storedBytes, err := hex.DecodeString(storedHex)
	if err != nil {
		return false, err
	}
	computedBytes, err := hex.DecodeString(computedHex)
	if err != nil {
		return false, err
	}

	// Constant-time compare
	return hmac.Equal(storedBytes, computedBytes), nil
}

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
