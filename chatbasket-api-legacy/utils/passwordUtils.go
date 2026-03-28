package utils

import (
	"chatbasket-api/model"

	"github.com/alexedwards/argon2id"
)

// HashPassword hashes the password using argon2id and returns the encoded hash string
func HashPassword(password string) (string, *model.AppError) {
	if !ValidateSixDigitCode(password) {
		return "", &model.AppError{Type: "validation_error", Message: "password must be a 6-digit number"}
	}
	// OWASP Recommended: 19 MiB memory, 2 iterations, 1 parallelism.
	// We use slightly higher defaults: 32MB memory, 2 iterations, 2 parallelism
	params := &argon2id.Params{
		Memory:      32 * 1024, // 32 MB
		Iterations:  2,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
	hash, err := argon2id.CreateHash(password, params)
	if err != nil {
		return "", &model.AppError{Type: "hashing_error", Message: err.Error()}
	}
	return hash, nil
}

// VerifyPassword compares a plain password with the hashed password from the DB
func VerifyPassword(plainPassword, hashedPassword string) (bool, *model.AppError) {
	if !ValidateSixDigitCode(plainPassword) {
		return false, &model.AppError{Type: "validation_error", Message: "password must be a 6-digit number"}
	}
	match, err := argon2id.ComparePasswordAndHash(plainPassword, hashedPassword)
	if err != nil {
		return false, &model.AppError{Type: "hashing_error", Message: err.Error()}
	}
	return match, nil
}

// ValidateSixDigitCode checks if the string is exactly 6 numeric digits
func ValidateSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	// Optimization: Iterate bytes directly. Digits 0-9 are single-byte ASCII.
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}
