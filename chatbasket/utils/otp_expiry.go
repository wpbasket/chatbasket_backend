package utils

import (
	"time"
)

// IsExpiredOTP checks if the OTP is expired based on createdAt and duration (in minutes)
func IsExpiredOTP(createdAt time.Time, validMinutes int) bool {
	expiry := createdAt.Add(time.Duration(validMinutes) * time.Minute)
	return time.Now().After(expiry)
}
