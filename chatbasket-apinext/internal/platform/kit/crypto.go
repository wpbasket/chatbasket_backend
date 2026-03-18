package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// ComputeHMAC is a generic helper to compute HMAC-SHA256 hex string, ported from utils/hashingTextUtils.go
func ComputeHMAC(data string, secretKey []byte) (string, error) {
	mac := hmac.New(sha256.New, secretKey)
	if _, err := mac.Write([]byte(data)); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyHMAC compares a data string against stored HMAC hex securely, ported from utils/hashingTextUtils.go
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
