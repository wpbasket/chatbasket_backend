package kit

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// DerefTime safely dereferences a *time.Time to time.Time.
func DerefTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

func LoadKeyFromEnvInByte(envVar string) ([]byte, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return nil, fmt.Errorf("missing env var: %s", envVar)
	}
	key, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %v", err)
	}
	return key, nil
}

func LoadKeyFromEnv(envVar string) (string, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return "", fmt.Errorf("missing env var: %s", envVar)
	}
	return val, nil
}

func StringToUUID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, err
	}
	return parsed, nil
}

const DefaultPostgresMaxConnLifetime = 30 * time.Minute

func ExtractUserID(c *echo.Context) (UserId, error) {
	userId, okStr := (*c).Get("userId").(string)
	uuidUserId, okUUID := (*c).Get("uuidUserId").(uuid.UUID)
	if !okStr || userId == "" || !okUUID {
		return UserId{}, NewError(http.StatusUnauthorized, "unauthorized", "User id is missing or invalid")
	}
	return UserId{
		StringUserId: userId,
		UuidUserId:   uuidUserId,
	}, nil
}

func ExtractSessionCreatedAt(c *echo.Context) (time.Time, error) {
	sessionCreatedAt, ok := (*c).Get("sessionCreatedAt").(time.Time)
	if !ok {
		return time.Time{}, NewError(http.StatusUnauthorized, "unauthorized", "Session creation time is missing")
	}
	return sessionCreatedAt, nil
}

func ExtractSessionUUID(c *echo.Context) (uuid.UUID, error) {
	sessionUUID, ok := (*c).Get("sessionUUID").(uuid.UUID)
	if !ok || sessionUUID == uuid.Nil {
		return uuid.Nil, NewError(http.StatusUnauthorized, "unauthorized", "Session UUID is missing or invalid")
	}
	return sessionUUID, nil
}

