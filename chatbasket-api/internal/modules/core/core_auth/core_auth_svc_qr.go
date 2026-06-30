package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// QRInitiatePayload is the response to initiating a QR login.
type QRInitiatePayload struct {
	QRToken   string `json:"qr_token"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

// QRApproveResponse indicates approval status.
type QRApproveResponse struct {
	Status bool `json:"status"`
}

// QRInitiate generates a new QR token, saves it in the database, and returns the signed token.
func (s *AuthService) QRInitiate(ctx context.Context) (*QRInitiatePayload, error) {
	qrToken, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate token")
	}

	// 5 minutes expiry
	expiresAt := time.Now().Add(5 * time.Minute)

	id, err := s.PostgresQuerier.CreateQRLoginRequest(ctx, core_auth_store.CreateQRLoginRequestParams{
		ID:        qrToken,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create QR request")
	}

	// Compute HMAC signature of the UUID string
	signature, err := kit.ComputeHMAC(id.String(), s.AuthSecret, false, nil)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to sign token")
	}

	return &QRInitiatePayload{
		QRToken:   fmt.Sprintf("%s.%s", id.String(), signature),
		ExpiresIn: 300,
	}, nil
}

// ParseAndVerifyQRToken verifies the signature of a token and returns the raw UUID.
func (s *AuthService) ParseAndVerifyQRToken(tokenStr string) (uuid.UUID, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return uuid.Nil, kit.NewError(http.StatusBadRequest, "invalid_token", "Malformed QR token")
	}
	tokenUUIDStr := parts[0]
	signature := parts[1]

	// Re-verify signature
	expectedSig, err := kit.ComputeHMAC(tokenUUIDStr, s.AuthSecret, false, nil)
	if err != nil {
		return uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify signature")
	}

	if expectedSig != signature {
		return uuid.Nil, kit.NewError(http.StatusUnauthorized, "invalid_token", "Invalid QR token signature")
	}

	parsedUUID, err := uuid.Parse(tokenUUIDStr)
	if err != nil {
		return uuid.Nil, kit.NewError(http.StatusBadRequest, "invalid_token", "Invalid token UUID format")
	}

	return parsedUUID, nil
}

// QRApprove verifies the token signature and links the authenticated user's ID to the QR login request.
// It executes a Postgres NOTIFY upon successful update.
func (s *AuthService) QRApprove(ctx context.Context, userID uuid.UUID, qrTokenStr string) (*QRApproveResponse, error) {
	token, err := s.ParseAndVerifyQRToken(qrTokenStr)
	if err != nil {
		return nil, err
	}

	_, err = s.PostgresQuerier.ApproveQRLogin(ctx, core_auth_store.ApproveQRLoginParams{
		ID:         token,
		AuthUserID: &userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to approve QR request")
	}

	// Notify Postgres
	if err := s.PostgresQuerier.NotifyQREvent(ctx, fmt.Sprintf("%s_approve", token.String())); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to notify QR event")
	}

	return &QRApproveResponse{Status: true}, nil
}

// QRCallback verifies the token signature and exchanges the APPROVED token for a real session.
func (s *AuthService) QRCallback(ctx context.Context, qrTokenStr string, platform string) (*SessionResponse, error) {
	token, err := s.ParseAndVerifyQRToken(qrTokenStr)
	if err != nil {
		return nil, err
	}

	userID, err := s.PostgresQuerier.ExchangeQRLogin(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_token", "QR Token expired or not approved")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to exchange QR request")
	}

	if userID == nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Missing user ID on approved token")
	}

	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, *userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to get user details")
	}

	// IP and UserAgent are optional for QR login since it's proxy authenticated via mobile
	platformStr := "web"
	session, err := s.CreateSessionFlow(ctx, *userID, &platformStr, nil)
	if err != nil {
		return nil, err
	}

	// Fetch keys revision
	keysRevision, err := s.GetKeysRevision(ctx, *userID)
	if err != nil {
		keysRevision = 0
	}

	return &SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         session.Token,
		SessionExpiry:     session.ExpiresAt,
		IsPrimary:         session.IsPrimary,
		PrimaryDeviceName: session.PrimaryDeviceName,
		PrimaryKey:        session.PrimaryKey,
		KeysRevision:      keysRevision,
	}, nil
}

// CleanupQRLoginRequests forcefully deletes expired tokens.
func (s *AuthService) CleanupQRLoginRequests(ctx context.Context) error {
	return s.PostgresQuerier.CleanupExpiredQRLoginRequests(ctx)
}
