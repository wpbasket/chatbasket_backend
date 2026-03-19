package middleware

import (
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

// AuthSessionProvider defines the methods required by the middleware to verify sessions.
// This interface allows the platform middleware to be decoupled from the auth module.
type AuthSessionProvider interface {
	GetSessionByToken(ctx context.Context, params postgresgen.GetSessionByTokenParams) (postgresgen.Session, error)
	GetAuthUserByID(ctx context.Context, userID uuid.UUID) (postgresgen.AuthUser, error)
	GetAuthSecret() []byte
}

// AuthSessionMiddleware verifies the session token and populates the context.
// Ported from legacy middleware/session.go with modular adjustments.
func AuthSessionMiddleware(authProvider AuthSessionProvider, requireVerified bool) echo.MiddlewareFunc {
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
				return c.JSON(http.StatusUnauthorized, kit.ApiError{
					Code:    http.StatusUnauthorized,
					Type:    "missing_auth",
					Message: "Missing session ID or User ID",
				})
			}

			// 2. Parse User ID to UUID
			uuidVal, err := kit.StringToUUID(userId)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, kit.ApiError{
					Code:    http.StatusUnauthorized,
					Type:    "invalid_user_id",
					Message: "Invalid user format",
				})
			}

			// 3. Compute HMAC and Verify Session via Store
			tokenHash, err := kit.ComputeHMAC(sessionId, authProvider.GetAuthSecret(), true, new(uuidVal.String()))
			if err != nil {
				return c.JSON(http.StatusInternalServerError, kit.ApiError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_error",
					Message: "Failed to process session token",
				})
			}

			ctx := c.Request().Context()
			session, err := authProvider.GetSessionByToken(ctx, postgresgen.GetSessionByTokenParams{
				TokenHash:  tokenHash,
				AuthUserID: uuidVal,
			})

			// Check if session found and not expired
			if err != nil || session.ExpiresAt.Time.Before(time.Now()) {
				return c.JSON(http.StatusUnauthorized, kit.ApiError{
					Code:    http.StatusUnauthorized,
					Type:    "session_invalid",
					Message: "Invalid or expired session",
				})
			}

			// 4. Get User details
			authUser, err := authProvider.GetAuthUserByID(ctx, uuidVal)
			if err != nil {
				if err == pgx.ErrNoRows {
					return c.JSON(http.StatusUnauthorized, kit.ApiError{
						Code:    http.StatusUnauthorized,
						Type:    "user_not_found",
						Message: "User not found",
					})
				}
				return c.JSON(http.StatusInternalServerError, kit.ApiError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_server_error",
					Message: "Failed to fetch user: " + kit.GetPostgresError(err).Message,
				})
			}

			// 5. Check Verification (if required)
			if requireVerified && !authUser.IsEmailVerified {
				return c.JSON(http.StatusForbidden, kit.ApiError{
					Code:    http.StatusForbidden,
					Type:    "unverified_email",
					Message: "Email must be verified to perform this action",
				})
			}

			// ✅ Set context for handler access
			c.Set("uuidUserId", authUser.ID)
			c.Set("userId", authUser.ID.String())
			c.Set("sessionId", sessionId)
			c.Set("platform", platform)
			c.Set("email", authUser.Email)
			c.Set("isPrimary", session.IsCentral)

			return next(c)
		}
	}
}
