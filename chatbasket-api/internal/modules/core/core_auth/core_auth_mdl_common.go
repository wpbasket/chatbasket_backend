package core_auth

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// LogoutPayload for logout requests (works for both public and personal modes)
type LogoutPayload struct {
	AllSessions bool `json:"all_sessions"`
}

// 🔄 Two-Step Password Update Models

// RequestUpdateOTPPayload is used to request an OTP for update operations
type RequestUpdateOTPPayload struct {
	UpdateType string `json:"updateType"` // "password_update" or "email_update"
}

// ConfirmPasswordUpdatePayload is used to confirm password update with OTP
type ConfirmPasswordUpdatePayload struct {
	UpdateID    string `json:"updateId"`    // UUID from RequestUpdateOTP
	Otp         string `json:"otp"`         // OTP code
	NewPassword string `json:"newPassword"` // New password to set
}

// 🔄 Two-Step Email Update Models

// RequestEmailUpdatePayload is used to request email update
type RequestEmailUpdatePayload struct {
	NewEmail string `json:"newEmail"` // New email address
	Password string `json:"password"` // Current password for verification
}

// ConfirmEmailUpdatePayload is used to confirm email update with OTP
type ConfirmEmailUpdatePayload struct {
	UpdateID string `json:"updateId"` // UUID from RequestEmailUpdate
	Otp      string `json:"otp"`      // OTP code
}

// Web cookie attributes shared by login/signup (set) and logout/delete (clear).
// The clear path must mirror every attribute of the set path (including
// SameSite) or browsers may keep the old cookie on some setups.
const (
	webCookieDomain    = "chatbasket.live"
	webCookieLocalHost = "localhost:8081"
)

// 🔄 Two-Step Account Deletion Models

// DeletePersonalAccountPayload is used to permanently delete a personal account with OTP
type DeletePersonalAccountPayload struct {
	UpdateID string `json:"updateId"` // UUID from RequestUpdateOTP
	Otp      string `json:"otp"`      // OTP code
}


// cookieSecurity derives domain + secure flag from the request origin.
// Local frontend (localhost:8081) uses host-default insecure cookies;
// production uses the shared domain with Secure.
func cookieSecurity(origin string) (domain string, secure bool) {
	if strings.Contains(origin, webCookieLocalHost) {
		return "", false
	}
	return webCookieDomain, true
}

// clearAuthCookies expires sessionId + userId with the same attributes the
// login path sets (Path, HttpOnly, Secure, Domain, SameSite). Attributes must
// match or the browser treats the clear as a different cookie and keeps the
// old session cookie.
func clearAuthCookies(c *echo.Context, origin string) {
	domain, secure := cookieSecurity(origin)
	c.SetCookie(&http.Cookie{
		Name:     "sessionId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// clearConnectAuthCookies is the Connect-RPC (raw header) variant of
// clearAuthCookies with identical attributes.
func clearConnectAuthCookies(header http.Header, origin string) {
	domain, secure := cookieSecurity(origin)
	sessionCookie := &http.Cookie{
		Name:     "sessionId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	userCookie := &http.Cookie{
		Name:     "userId",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		Domain:   domain,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	header.Add("Set-Cookie", sessionCookie.String())
	header.Add("Set-Cookie", userCookie.String())
}
