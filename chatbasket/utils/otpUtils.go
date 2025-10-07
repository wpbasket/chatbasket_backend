package utils

import (
	"crypto/rand"
	"time"
	"github.com/alexedwards/argon2id"
)

// GenerateOTP generates a secure 6-digit numeric OTP
func GenerateOTP() (string, error) {
	digits := "0123456789"
	otp := make([]byte, 6)
	_, err := rand.Read(otp)
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ {
		otp[i] = digits[otp[i]%10]
	}
	return string(otp), nil
}

// HashOTP hashes the OTP using argon2id and returns the encoded hash string
func HashOTP(otp string) (string, error) {
	return argon2id.CreateHash(otp, argon2id.DefaultParams)
}

// VerifyOTP compares a plain OTP with the hashed OTP from the DB
func VerifyOTP(plainOTP, hashedOTP string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plainOTP, hashedOTP)
}


// IsExpiredOTP checks if the OTP is expired based on createdAt and duration (in minutes)
func IsExpiredOTP(createdAt time.Time, validMinutes int) bool {
	expiry := createdAt.Add(time.Duration(validMinutes) * time.Minute)
	return time.Now().After(expiry)
}