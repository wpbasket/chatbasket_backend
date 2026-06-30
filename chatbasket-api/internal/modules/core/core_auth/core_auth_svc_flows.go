package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"fmt"
	htmlpkg "html"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SendVerificationOTPFlow generates OTP, hashes it, stores in DB, and sends email.
func (s *AuthService) SendVerificationOTPFlow(ctx context.Context, userID uuid.UUID, email, otpType string) error {
	// Generate OTP
	otp, err := GenerateOTP()
	if err != nil {
		log.Printf("Failed to generate OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate OTP: "+err.Error())
	}

	hashedOTP, err := HashOTP(otp)
	if err != nil {
		log.Printf("Failed to hash OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash OTP")
	}

	// Store in DB (UPSERT on id=userID handles strict 1:1)
	// ExpiresAt column removed; logic relies on CreatedAt + 3 mins
	_, err = s.PostgresQuerier.CreateVerificationCode(ctx, core_auth_store.CreateVerificationCodeParams{
		ID:       userID, // Using UserID as the PK directly
		Email:    email,
		CodeHash: hashedOTP,
		Type:     otpType,
	})
	if err != nil {
		log.Printf("Failed to store OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to save OTP")
	}

	// Build a transactional-grade email: branded HTML + matching plain text.
	subject, htmlBody, textBody := buildOTPEmail(otpType, otp)
	refID := "otp-" + otpType

	if err := clients.SendEmailExt([]string{email}, subject, htmlBody, textBody, refID); err != nil {
		log.Printf("Failed to send OTP email: %v", err.Error())
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to send email: "+err.Error())
	}
	return nil
}

// otpCopy holds the per-flow subject and short, human-readable description
// used by buildOTPEmail. Centralising the wording here keeps every OTP mail
// consistent in tone and structure.
type otpCopy struct {
	Subject  string // mail subject line
	Headline string // bold heading shown above the code
	Purpose  string // single sentence explaining what the code is for
}

// otpCopies maps an internal OTP type to its user-facing wording. Any unknown
// type falls back to a safe generic copy via buildOTPEmail.
var otpCopies = map[string]otpCopy{
	"login": {
		Subject:  "Your ChatBasket login code",
		Headline: "Sign-in verification code",
		Purpose:  "Use this code to finish signing in to ChatBasket.",
	},
	"email_verification": {
		Subject:  "Verify your ChatBasket email",
		Headline: "Email verification code",
		Purpose:  "Use this code to confirm your email address on ChatBasket.",
	},
	"password_reset": {
		Subject:  "Reset your ChatBasket password",
		Headline: "Password reset code",
		Purpose:  "Use this code to reset the password on your ChatBasket account.",
	},
	// password_update — sent during the in-session "change password" flow.
	// Distinct from password_reset (which is the forgot-password / out-of-session
	// flow) so the user immediately understands why they're seeing this email.
	"password_update": {
		Subject:  "Confirm your ChatBasket password change",
		Headline: "Password change verification code",
		Purpose:  "Use this code to confirm the password change on your ChatBasket account.",
	},
	// email_update — sent to the *new* address during the "change email" flow.
	// The body is deliberately explicit ("new email") so the recipient can spot
	// an unauthorised change request even if they don't recognise the sender.
	"email_update": {
		Subject:  "Confirm your new ChatBasket email",
		Headline: "Email change verification code",
		Purpose:  "Use this code to confirm the new email address on your ChatBasket account.",
	},
}

// buildOTPEmail renders a transactional OTP email and returns its subject,
// HTML body and plain-text alternative. The output is intentionally:
//
//   - Branded — the message clearly identifies ChatBasket as the sender, so
//     it doesn't look like a phishing skeleton and isn't classified as one
//     by Gmail's ML filters.
//   - Self-explanatory — a short sentence tells the user exactly what the
//     code is for and that it expires in 3 minutes, which materially lifts
//     engagement (and therefore inbox placement over time).
//   - Defensive — both the HTML and the plain-text variant include the
//     "ignore this email if you didn't request it" security disclaimer
//     that all major mail providers expect for OTP traffic.
//   - Predictable — HTML escaping prevents an attacker-controlled subject
//     or OTP from injecting markup, even though both are server-generated
//     today.
func buildOTPEmail(otpType, otp string) (subject, htmlBody, textBody string) {
	c, ok := otpCopies[otpType]
	if !ok {
		c = otpCopy{
			Subject:  "Your ChatBasket verification code",
			Headline: "Verification code",
			Purpose:  "Use this code to continue with your ChatBasket request.",
		}
	}

	safeOTP := htmlEscape(otp)
	safeHeadline := htmlEscape(c.Headline)
	safePurpose := htmlEscape(c.Purpose)

	htmlBody = fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
</head>
<body style="margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2933;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="padding:24px 0;">
    <tr><td align="center">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;border-radius:12px;border:1px solid #e4e7eb;">
        <tr><td style="padding:32px 32px 8px 32px;">
          <p style="margin:0 0 4px 0;font-size:14px;color:#52606d;letter-spacing:0.02em;">ChatBasket</p>
          <h1 style="margin:0;font-size:20px;color:#1f2933;">%s</h1>
        </td></tr>
        <tr><td style="padding:8px 32px 0 32px;">
          <p style="margin:0;font-size:15px;line-height:1.6;color:#3e4c59;">Hi,</p>
          <p style="margin:12px 0 0 0;font-size:15px;line-height:1.6;color:#3e4c59;">%s</p>
        </td></tr>
        <tr><td align="center" style="padding:24px 32px;">
          <div style="display:inline-block;padding:16px 28px;border:1px solid #d9e2ec;border-radius:8px;font-family:'SFMono-Regular',Consolas,Menlo,monospace;font-size:28px;font-weight:700;letter-spacing:8px;color:#102a43;">%s</div>
        </td></tr>
        <tr><td style="padding:0 32px 28px 32px;">
          <p style="margin:0;font-size:14px;line-height:1.6;color:#52606d;">This code expires in <strong>3 minutes</strong>. For your security, never share it with anyone — ChatBasket staff will never ask for it.</p>
          <p style="margin:16px 0 0 0;font-size:14px;line-height:1.6;color:#52606d;">If you didn't request this code, you can safely ignore this email; your account remains secure.</p>
        </td></tr>
        <tr><td style="padding:24px 32px 28px 32px;border-top:1px solid #e4e7eb;">
          <p style="margin:0;font-size:12px;line-height:1.6;color:#7b8794;">You're receiving this email because a verification was requested for this address on ChatBasket.</p>
          <p style="margin:8px 0 0 0;font-size:12px;line-height:1.6;color:#7b8794;">This is an automated message, please do not reply. Need help? Contact <a href="mailto:support@chatbasket.live" style="color:#2563eb;text-decoration:underline;">support@chatbasket.live</a></p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeHeadline, safeHeadline, safePurpose, safeOTP)

	textBody = fmt.Sprintf(`ChatBasket — %s

Hi,

%s

Your code: %s

This code expires in 3 minutes. For your security, never share it with anyone
— ChatBasket staff will never ask for it.

If you didn't request this code, you can safely ignore this email; your
account remains secure.

—
You're receiving this email because a verification was requested for this
address on ChatBasket. This is an automated message, please do not reply.
Need help? Contact support@chatbasket.live
`, c.Headline, c.Purpose, otp)

	return c.Subject, htmlBody, textBody
}

// htmlEscape is a thin wrapper around html.EscapeString. It is exposed as a
// named helper so it's obvious at the call site that the value being
// rendered is untrusted (or could become untrusted).
func htmlEscape(s string) string { return htmlpkg.EscapeString(s) }

// VerifyOTPFlow retrieves code by UserID, checks created_at expiry (3 mins), validates hash, and consumes code.
func (s *AuthService) VerifyOTPFlow(ctx context.Context, userID uuid.UUID, secret, otpType string) (bool, error) {
	// 1. Check if blocked due to brute-force protection
	if err := s.CheckOTPVerifyRateLimit(ctx, userID); err != nil {
		return false, err
	}

	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: otpType,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// No code found
			return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid OTP")
		}
		return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "DB Error")
	}

	// Check Expiry (Created At + 3 Minutes)
	// Passing 3 as validity duration in minutes
	if IsExpiredOTP(record.CreatedAt, 3) {
		return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "OTP has expired")
	}

	// Verify Hash
	match, err := VerifyOTP(secret, record.CodeHash)
	if err != nil {
		return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Hash verification error")
	}

	if match {
		// 2. Success: Reset error counter and consume the code
		if err := s.ResetVerifyErrors(ctx, userID); err != nil {
			return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to reset verification errors")
		}
		if err := s.PostgresQuerier.DeleteVerificationCode(ctx, record.ID); err != nil {
			return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete verification code")
		}
		return true, nil
	}

	// 3. Failure: Record the error and block if threshold reached
	if err := s.RecordVerifyError(ctx, userID); err != nil {
		return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to record verification error")
	}
	return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid OTP")
}

type SessionResult struct {
	Token             string
	ExpiresAt         string
	IsPrimary         bool
	PrimaryDeviceName string
	PrimaryKey        string
}

// CreateSessionFlow generates token, hashes, stores session, returns token + expiry string.
func (s *AuthService) CreateSessionFlow(ctx context.Context, userID uuid.UUID, platform, deviceToken *string) (*SessionResult, error) {
	tokenEnv := uuid.New().String()
	tokenHash, err := kit.ComputeHMAC(tokenEnv, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Hash error")
	}

	// 3 years expiry
	expiresAt := time.Now().Add(3 * 365 * 24 * time.Hour)
	sid, _ := uuid.NewV7()

	// Logic: If user has NO primary device, and this is a native platform, make this the primary device.
	isPrimary := false
	primaryDeviceName := ""
	primaryKey := ""

	// Check for existing primary device
	existingPrimary, err := s.PostgresQuerier.GetCentralSession(ctx, userID)
	if err == nil {
		// Existing primary device found
		if existingPrimary.DeviceName != nil {
			primaryDeviceName = *existingPrimary.DeviceName
		}
		if existingPrimary.E2eePublicKey != nil {
			primaryKey = *existingPrimary.E2eePublicKey
		}
	} else {
		// No primary device found, auto-promote ANY platform for now (Temporary Fix for Web Messaging)
		isPrimary = true
	}

	_, err = s.PostgresQuerier.CreateSession(ctx, core_auth_store.CreateSessionParams{
		ID:          sid,
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt,
		Platform:    platform,
		DeviceToken: deviceToken,
		DeviceName:  nil, // To be set later via settings
		IsCentral:   isPrimary,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create session: "+err.Error())
	}

	return &SessionResult{
		Token:             tokenEnv,
		ExpiresAt:         expiresAt.Format(time.RFC3339),
		IsPrimary:         isPrimary,
		PrimaryDeviceName: primaryDeviceName,
		PrimaryKey:        primaryKey,
	}, nil
}
