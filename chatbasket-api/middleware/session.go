package middleware

import (
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/services"
	"chatbasket-api/utils"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

func AuthSessionMiddleware(authService *services.AuthService, requireVerified bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
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
				// Used by mobile WS clients that can't set headers on WebSocket upgrade.
				platform = "native"
				parts := strings.SplitN(tokenParam, ":", 2)
				if len(parts) == 2 {
					sessionId, userId = parts[0], parts[1]
				}
			} else {
				// Request from web - extract from httpOnly cookies
				platform = "web"

				// Extract sessionId from cookie
				sessionCookie, err := c.Cookie("sessionId")
				if err == nil {
					sessionId = sessionCookie.Value
				}

				// Extract userId from cookie
				userCookie, err := c.Cookie("userId")
				if err == nil {
					userId = userCookie.Value
				}
			}

			// 🔒 Check missing auth
			if sessionId == "" || userId == "" {
				// log.Printf("401 returned: Missing session ID or User ID. sessionId='%s', userId='%s', platform='%s'", sessionId, userId, platform)
				return c.JSON(http.StatusUnauthorized, model.SessionError{
					Code:    http.StatusUnauthorized,
					Type:    "missing_auth",
					Message: "Missing session ID or User ID",
				})
			}

			// 🔐 Verify session using internal Auth Service (HMAC + User Check)
			// Secret Key is injected via AuthService
			secretKey := authService.AuthSecret

			// 1. Hash the sessionId (token)
			tokenHash, err := utils.ComputeHMAC(sessionId, secretKey)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, model.SessionError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_error",
					Message: "Failed to process session token",
				})
			}

			// 2. Parse User ID to UUID for DB check
			uuidVal, err := utils.StringToUUID(userId)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, model.SessionError{
					Code:    http.StatusUnauthorized,
					Type:    "invalid_user_id",
					Message: "Invalid user format",
				})
			}

			// 3. Get Session details (Validate token & Get is_central flag)
			ctx := c.Request().Context()
			session, err := authService.AuthQueries.GetSessionByToken(ctx, auth.GetSessionByTokenParams{
				TokenHash:  tokenHash,
				AuthUserID: uuidVal,
			})

			// Check if session found and not expired
			if err != nil || session.ExpiresAt.Time.Before(time.Now()) {
				// If error is no rows, it means invalid token/user combo
				return c.JSON(http.StatusUnauthorized, model.SessionError{
					Code:    http.StatusUnauthorized,
					Type:    "session_invalid",
					Message: "Invalid or expired session",
				})
			}

			// 4. Get User details
			authUser, err := authService.AuthQueries.GetAuthUserByID(ctx, uuidVal)
			if err != nil {
				if err == pgx.ErrNoRows {
					return c.JSON(http.StatusUnauthorized, model.SessionError{
						Code:    http.StatusUnauthorized,
						Type:    "user_not_found",
						Message: "User not found",
					})
				}
				return c.JSON(http.StatusInternalServerError, model.SessionError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_server_error",
					Message: "Failed to fetch user: " + utils.GetPostgresError(err).Message,
				})
			}

			// 5. Check Verification (if required)
			// Removed as per user request (was not in original implementation)
			// if requireVerified && !authUser.IsEmailVerified { ... }

			// ✅ Set context for handler access
			c.Set("uuidUserId", authUser.ID) // auth_users.ID is same as users.ID
			c.Set("userId", authUser.ID.String())
			c.Set("sessionId", sessionId) // Context keeps original input sessionId
			c.Set("platform", platform)
			c.Set("email", authUser.Email)
			c.Set("isPrimary", session.IsCentral) // Trusted flag from DB

			return next(c)
		}
	}
}
