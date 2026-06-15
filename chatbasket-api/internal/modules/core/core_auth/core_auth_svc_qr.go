package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// QRInitiatePayload is the response to initiating a QR login.
type QRInitiatePayload struct {
	QRToken   string `json:"qr_token"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

// QRSignalPayload represents SDP exchanges for the WebRTC connection.
type QRSignalPayload struct {
	QRToken   string `json:"qr_token" validate:"required,uuid"`
	Role      string `json:"role" validate:"required,oneof=browser mobile"`
	SDP       string `json:"sdp"`
	Candidate string `json:"candidate"`
}

// QRSignalResponse is returned when asking for a signal.
type QRSignalResponse struct {
	SDP        string   `json:"sdp,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
	Status     string   `json:"status"`
}

// QRApproveResponse indicates approval status.
type QRApproveResponse struct {
	Status bool `json:"status"`
}

// QRCallbackResponse returns the user details and session cookie after successful login.
type QRCallbackResponse struct {
	AuthUser      *core_auth_store.AuthUser
	SessionID     string
	SessionExpiry string
}

func parseQRCandidates(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rawCandidates []json.RawMessage
	if err := json.Unmarshal(raw, &rawCandidates); err != nil {
		return nil, err
	}
	candidates := make([]string, 0, len(rawCandidates))
	for _, candidate := range rawCandidates {
		if len(candidate) == 0 || string(candidate) == "null" {
			continue
		}
		candidates = append(candidates, string(candidate))
	}
	return candidates, nil
}

// QRInitiate generates a new QR token and saves it in the database.
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

	return &QRInitiatePayload{
		QRToken:   id.String(),
		ExpiresIn: 300,
	}, nil
}

// QRSignal handles WebRTC SDP offers and answers.
// It executes a Postgres NOTIFY upon successful update.
func (s *AuthService) QRSignal(ctx context.Context, payload *QRSignalPayload) (*QRSignalResponse, error) {
	token, err := uuid.Parse(payload.QRToken)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid QR Token format")
	}

	if payload.Role == "browser" {
		if payload.Candidate != "" {
			if !json.Valid([]byte(payload.Candidate)) {
				return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid ICE candidate")
			}
			_, err := s.PostgresQuerier.AddQRLoginBrowserCandidate(ctx, core_auth_store.AddQRLoginBrowserCandidateParams{
				ID:      token,
				Column2: []byte(payload.Candidate),
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update ICE candidate")
			}
			if err := s.PostgresQuerier.NotifyQREvent(ctx, fmt.Sprintf("%s_signal", token.String())); err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to notify QR event")
			}
			return &QRSignalResponse{Status: "CANDIDATE_SAVED"}, nil
		}
		if payload.SDP != "" {
			// Browser setting offer
			_, err := s.PostgresQuerier.UpdateQRLoginSignalOffer(ctx, core_auth_store.UpdateQRLoginSignalOfferParams{
				ID:          token,
				SignalOffer: &payload.SDP,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update offer")
			}

			// Notify Postgres
			if err := s.PostgresQuerier.NotifyQREvent(ctx, fmt.Sprintf("%s_signal", token.String())); err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to notify QR event")
			}

			return &QRSignalResponse{Status: "OFFER_SAVED"}, nil
		} else {
			// Browser getting answer
			req, err := s.PostgresQuerier.GetQRLoginRequest(ctx, token)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to retrieve answer")
			}
			ans := ""
			if req.SignalAnswer != nil {
				ans = *req.SignalAnswer
			}
			candidates, err := parseQRCandidates(req.MobileCandidates)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to retrieve ICE candidates")
			}
			return &QRSignalResponse{SDP: ans, Candidates: candidates, Status: "OK"}, nil
		}
	} else if payload.Role == "mobile" {
		if payload.Candidate != "" {
			if !json.Valid([]byte(payload.Candidate)) {
				return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid ICE candidate")
			}
			_, err := s.PostgresQuerier.AddQRLoginMobileCandidate(ctx, core_auth_store.AddQRLoginMobileCandidateParams{
				ID:      token,
				Column2: []byte(payload.Candidate),
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update ICE candidate")
			}
			if err := s.PostgresQuerier.NotifyQREvent(ctx, fmt.Sprintf("%s_signal", token.String())); err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to notify QR event")
			}
			return &QRSignalResponse{Status: "CANDIDATE_SAVED"}, nil
		}
		if payload.SDP != "" {
			// Mobile setting answer
			_, err := s.PostgresQuerier.UpdateQRLoginSignalAnswer(ctx, core_auth_store.UpdateQRLoginSignalAnswerParams{
				ID:           token,
				SignalAnswer: &payload.SDP,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update answer")
			}

			// Notify Postgres
			if err := s.PostgresQuerier.NotifyQREvent(ctx, fmt.Sprintf("%s_signal", token.String())); err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to notify QR event")
			}

			return &QRSignalResponse{Status: "ANSWER_SAVED"}, nil
		} else {
			// Mobile getting offer
			req, err := s.PostgresQuerier.GetQRLoginRequest(ctx, token)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
				}
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to retrieve offer")
			}
			offer := ""
			if req.SignalOffer != nil {
				offer = *req.SignalOffer
			}
			candidates, err := parseQRCandidates(req.BrowserCandidates)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to retrieve ICE candidates")
			}
			return &QRSignalResponse{SDP: offer, Candidates: candidates, Status: "OK"}, nil
		}
	}

	return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid role")
}

// QRApprove links the authenticated user's ID to the QR login request.
// It executes a Postgres NOTIFY upon successful update.
func (s *AuthService) QRApprove(ctx context.Context, userID uuid.UUID, qrTokenStr string) (*QRApproveResponse, error) {
	token, err := uuid.Parse(qrTokenStr)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid QR Token format")
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

// QRCallback exchanges the APPROVED token for a real session.
func (s *AuthService) QRCallback(ctx context.Context, qrTokenStr string, platform string) (*QRCallbackResponse, error) {
	token, err := uuid.Parse(qrTokenStr)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid QR Token format")
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

	return &QRCallbackResponse{
		AuthUser:      &user,
		SessionID:     session.Token,
		SessionExpiry: session.ExpiresAt,
	}, nil
}

// CleanupQRLoginRequests forcefully deletes expired tokens.
func (s *AuthService) CleanupQRLoginRequests(ctx context.Context) error {
	return s.PostgresQuerier.CleanupExpiredQRLoginRequests(ctx)
}
