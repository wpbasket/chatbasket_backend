package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// ComputeHMAC computes an HMAC-SHA256 hex digest with optional salt.
func ComputeHMAC(data string, secretKey []byte, useSalt bool, salt *string) (string, error) {
	mac := hmac.New(sha256.New, secretKey)
	if _, err := mac.Write([]byte(data)); err != nil {
		return "", err
	}
	if useSalt && salt != nil {
		// Use a delimiter to prevent concatenation ambiguity
		if _, err := mac.Write([]byte("|" + *salt)); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyHMAC compares a data string against a stored HMAC hex digest in constant time.
func VerifyHMAC(data string, storedHex string, secretKey []byte, useSalt bool, salt *string) (bool, error) {
	computedHex, err := ComputeHMAC(data, secretKey, useSalt, salt)
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
