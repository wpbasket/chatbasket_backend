package middleware

import (
	"chatbasket/appwrite"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

func AppwriteSessionMiddleware(requireVerified bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			sessionID := strings.TrimPrefix(authHeader, "Bearer ")

			if sessionID == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Missing session ID")
			}

			appwriteSvc := appwrite.NewAppwriteServiceWithSession(
				os.Getenv("APPWRITE_ENDPOINT"),
				os.Getenv("APPWRITE_PROJECT_ID"),
				sessionID,
				os.Getenv("APPWRITE_DATABASE_ID"),
				os.Getenv("APPWRITE_USERS_COLLECTION_ID"),
				os.Getenv("APPWRITE_POSTS_COLLECTION_ID"),
				os.Getenv("APPWRITE_COMMENTS_COLLECTION_ID"),
				os.Getenv("APPWRITE_BLOCK_COLLECTION_ID"),
				os.Getenv("APPWRITE_LIKES_COLLECTION_ID"),
				os.Getenv("APPWRITE_FOLLOW_COLLECTION_ID"),
				os.Getenv("APPWRITE_REFRESH_TOKENS_COLLECTION_ID"),
			)

			account, err := appwriteSvc.Account.Get()
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or expired session")
			}

			if requireVerified && !account.EmailVerification {
				return echo.NewHTTPError(http.StatusForbidden, "Email not verified")
			}

			// Set user ID and Appwrite service to context
			c.Set("userID", account.Id)
			c.Set("appwriteSessionService", appwriteSvc)

			return next(c)
		}
	}
}
