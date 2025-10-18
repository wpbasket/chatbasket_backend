package middleware

import (
	"chatbasket/appwriteinternal"
	"chatbasket/model"
	"chatbasket/utils"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)


func AppwriteSessionMiddleware(requireVerified bool) echo.MiddlewareFunc {
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

			// 🔐 Initialize Appwrite session service
			appwriteService := appwriteinternal.NewAppwriteServiceSession(
				os.Getenv("APPWRITE_ENDPOINT"),
				os.Getenv("APPWRITE_PROJECT_ID"),
				os.Getenv("APPWRITE_API_KEY"),
			)

			account, err := appwriteService.Users.ListSessions(userId)
			if err != nil {
				statusCode := http.StatusInternalServerError
				if he, ok := err.(*echo.HTTPError); ok {
					statusCode = he.Code
				}

				return c.JSON(statusCode, model.SessionError{
					Code:    statusCode,
					Type:    "session_list_failed",
					Message: err.Error(),
				})
			}

			// ✅ Search session ID match
			var sessionFound bool
			for _, session := range account.Sessions {
				if session.Id == sessionId {
					sessionFound = true
					break
				}
			}

			if !sessionFound {
				// log.Printf("401 returned: Invalid session ID. userId='%s', sessionId='%s', platform='%s'", userId, sessionId, platform)
				return c.JSON(http.StatusUnauthorized, model.SessionError{
					Code:    http.StatusUnauthorized,
					Type:    "session_invalid",
					Message: "Invalid session ID",
				})
			}

			getEmail, err := appwriteService.Users.Get(userId)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, model.SessionError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_server_error",
					Message: err.Error(),
				})
			}

			// Set to context for handler access
			uuidUserId, err := utils.StringToUUID(userId)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, model.SessionError{
					Code:    http.StatusInternalServerError,
					Type:    "internal_server_error",
					Message: err.Error(),
				})
			}
			c.Set("uuidUserId", uuidUserId)
			c.Set("userId", userId)
			c.Set("sessionId", sessionId)
			c.Set("platform", platform)
			c.Set("email", getEmail.Email)

			return next(c)
		}
	}
}
