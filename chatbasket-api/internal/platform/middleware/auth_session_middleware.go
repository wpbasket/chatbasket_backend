package middleware

import (
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

// SessionInfo represents generic session data for the middleware.
type SessionInfo struct {
	ID        uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
	IsCentral bool
}

// UserInfo represents generic user data for the middleware.
type UserInfo struct {
	ID              uuid.UUID
	Email           string
	IsEmailVerified bool
}

// AuthSessionProvider defines the methods required by the middleware to verify sessions.
// This interface allows the platform middleware to be decoupled from the auth module.
type AuthSessionProvider interface {
	GetSessionByToken(ctx context.Context, tokenHash string, userID uuid.UUID) (*SessionInfo, error)
	GetAuthUserByID(ctx context.Context, userID uuid.UUID) (*UserInfo, error)
	GetAuthSecret() []byte
}

// AuthSessionMiddleware verifies the session token and populates the context.
// Ported from legacy middleware/session.go with modular adjustments.
func AuthSessionMiddleware(authProvider AuthSessionProvider, requireVerified bool, hub *websocket.WSHub) echo.MiddlewareFunc {
	closeSessionConnection := func(userID uuid.UUID, sessionID string) {
		if hub != nil && sessionID != "" {
			hub.CloseSessionConnection(userID, sessionID)
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			var sessionId, userId string
			var platform string

			// Check if Authorization header is present (native apps)
			authHeader := c.Request().Header.Get("Authorization")

			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				// Request from native app (iOS/Android)
				platform = "native"
				token := strings.TrimPrefix(authHeader, "Bearer ")
				parts := strings.SplitN(token, ":", 2)
				if len(parts) == 2 {
					sessionId, userId = parts[0], parts[1]
				}
			} else if tokenParam := c.QueryParam("token"); tokenParam != "" {
				// WebSocket auth via query param (?token=sessionId:userId)
				platform = "native"
				parts := strings.SplitN(tokenParam, ":", 2)
				if len(parts) == 2 {
					sessionId, userId = parts[0], parts[1]
				}
			} else {
				// Request from web - extract from httpOnly cookies
				platform = "web"

				sessionCookie, err := c.Cookie("sessionId")
				if err == nil {
					sessionId = sessionCookie.Value
				}

				userCookie, err := c.Cookie("userId")
				if err == nil {
					userId = userCookie.Value
				}
			}

			// 1. Check missing auth
			if sessionId == "" || userId == "" {
				return kit.NewError(http.StatusUnauthorized, "missing_auth", "Missing session ID or User ID")
			}

			// 2. Parse User ID to UUID
			uuidVal, err := kit.StringToUUID(userId)
			if err != nil {
				return kit.NewError(http.StatusUnauthorized, "invalid_user_id", "Invalid user format")
			}

			// 3. Compute HMAC and Verify Session via Store
			tokenHash, err := kit.ComputeHMAC(sessionId, authProvider.GetAuthSecret(), true, new(uuidVal.String()))
			if err != nil {
				return kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to process session token")
			}

			ctx := c.Request().Context()
			session, err := authProvider.GetSessionByToken(ctx, tokenHash, uuidVal)

			// Check if session found and not expired
			if err != nil {
				// Distinguish between legitimate auth errors and infrastructure errors
				if err == pgx.ErrNoRows {
					closeSessionConnection(uuidVal, sessionId)
					return kit.NewError(http.StatusUnauthorized, "session_invalid", "Invalid or expired session")
				}
				// Database connection error or other infrastructure issue
				return kit.NewError(http.StatusServiceUnavailable, "service_unavailable", "Unable to verify session: "+kit.GetPostgresError(err).Message)
			}

			if session.ExpiresAt.Before(time.Now()) {
				closeSessionConnection(uuidVal, sessionId)
				return kit.NewError(http.StatusUnauthorized, "session_invalid", "Session expired")
			}

			// 4. Get User details
			authUser, err := authProvider.GetAuthUserByID(ctx, uuidVal)
			if err != nil {
				if err == pgx.ErrNoRows {
					closeSessionConnection(uuidVal, sessionId)
					return kit.NewError(http.StatusUnauthorized, "user_not_found", "User not found")
				}
				return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to fetch user: "+kit.GetPostgresError(err).Message)
			}

			// 5. Check Verification (if required)
			if requireVerified && !authUser.IsEmailVerified {
				closeSessionConnection(uuidVal, sessionId)
				return kit.NewError(http.StatusForbidden, "unverified_email", "Email must be verified to perform this action")
			}

			// âœ… Set context for handler access
			c.Set("uuidUserId", authUser.ID)
			c.Set("userId", authUser.ID.String())
			c.Set("sessionId", sessionId)
			c.Set("sessionUUID", session.ID)
			c.Set("sessionCreatedAt", session.CreatedAt)
			c.Set("platform", platform)
			c.Set("email", authUser.Email)
			c.Set("isPrimary", session.IsCentral)

			return next(c)
		}
	}
}
