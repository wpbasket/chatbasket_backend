package kit

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

type ctxKey string

const (
	CtxSessionData ctxKey = "sessionData"
)

// SessionData holds the authenticated session context.
type SessionData struct {
	UserID           string
	UUIDUserID       uuid.UUID
	SessionID        string
	SessionUUID      uuid.UUID
	Email            string
	SessionCreatedAt time.Time
	Platform         string
	IsPrimary        bool
}

// GetConnectRpcUserID helper extracts user details from the context.
func GetConnectRpcUserID(ctx context.Context) (UserId, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return UserId{}, connect.NewError(connect.CodeUnauthenticated, errors.New("user context missing or invalid"))
	}
	return UserId{
		StringUserId: data.UserID,
		UuidUserId:   data.UUIDUserID,
	}, nil
}

// GetConnectRpcEmail helper extracts the authenticated email from the context.
func GetConnectRpcEmail(ctx context.Context) (string, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("email context missing or invalid"))
	}
	return data.Email, nil
}

// GetConnectRpcSessionID helper extracts the session token string from the context.
func GetConnectRpcSessionID(ctx context.Context) (string, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("session ID context missing or invalid"))
	}
	return data.SessionID, nil
}

// GetConnectRpcSessionUUID helper extracts the session database UUID from the context.
func GetConnectRpcSessionUUID(ctx context.Context) (uuid.UUID, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("session UUID context missing or invalid"))
	}
	return data.SessionUUID, nil
}

// GetConnectRpcSessionCreatedAt helper extracts the session creation time from the context.
func GetConnectRpcSessionCreatedAt(ctx context.Context) (time.Time, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return time.Time{}, connect.NewError(connect.CodeUnauthenticated, errors.New("session creation time context missing or invalid"))
	}
	return data.SessionCreatedAt, nil
}

// GetConnectRpcPlatform helper extracts the client platform type (web/native) from the context.
func GetConnectRpcPlatform(ctx context.Context) (string, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("platform context missing or invalid"))
	}
	return data.Platform, nil
}

// GetConnectRpcIsPrimary helper checks if this session is the primary central device session.
func GetConnectRpcIsPrimary(ctx context.Context) (bool, error) {
	data, ok := ctx.Value(CtxSessionData).(SessionData)
	if !ok {
		return false, connect.NewError(connect.CodeUnauthenticated, errors.New("primary status context missing or invalid"))
	}
	return data.IsPrimary, nil
}
