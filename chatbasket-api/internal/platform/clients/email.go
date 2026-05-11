package clients

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

var emailWG sync.WaitGroup

// EmailRelayPayload represents the payload sent to the email relay service.
//
// `Body`     - HTML body (required).
// `TextBody` - Optional plain-text alternative. When set, the relay attaches
// it as the text/plain part of a multipart/alternative message. When empty,
// the relay derives one automatically from the HTML, but supplying a
// hand-written version yields better looking plain-text clients and lower
// spam scores.
// `RefID`    - Optional per-message reference id, surfaced in the outgoing
// `X-Entity-Ref-ID` header. Use a category (e.g. "otp-login") to help
// deliverability and downstream tracing without leaking PII.
type EmailRelayPayload struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	TextBody string   `json:"text_body,omitempty"`
	RefID    string   `json:"ref_id,omitempty"`
}

// WaitEmails waits for all background email tasks to complete, ported from utils/emailUtils.go
func WaitEmails() {
	emailWG.Wait()
}

// SendEmail is a convenience wrapper that posts an HTML-only message to the
// relay. The relay will derive a plain-text alternative on the fly. Prefer
// SendEmailExt when you can provide a curated plain-text body and a stable
// RefID — that produces noticeably better deliverability.
func SendEmail(to []string, subject string, bodyHTML string) error {
	return SendEmailExt(to, subject, bodyHTML, "", "")
}

// SendEmailExt sends an email using the Heroku Relay in a "fire and forget"
// background task. `bodyText` and `refID` are optional and forwarded to the
// relay verbatim.
func SendEmailExt(to []string, subject, bodyHTML, bodyText, refID string) error {
	relayURL := os.Getenv("MAIL_RELAY_URL")
	relaySecret := os.Getenv("MAIL_RELAY_SECRET")

	if relayURL == "" || relaySecret == "" {
		slog.Warn("Mail relay configuration is missing", "url", relayURL)
		return nil // Return nil so flow continues, but log the error
	}

	// Fire and forget in a goroutine using Go 1.26 WaitGroup.Go()
	emailWG.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered from panic in email goroutine", "error", r)
			}
		}()

		reqBody, _ := json.Marshal(EmailRelayPayload{
			To:       to,
			Subject:  subject,
			Body:     bodyHTML,
			TextBody: bodyText,
			RefID:    refID,
		})

		// Use a 30s timeout for the background relay call
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequest("POST", relayURL, bytes.NewBuffer(reqBody))
		if err != nil {
			slog.Error("Failed to create email request", "error", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Relay-Secret", relaySecret)

		resp, err := client.Do(req)
		if err != nil {
			slog.Error("Email relay connection failed", "error", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			slog.Error("Email relay returned error status", "status", resp.StatusCode)
		} else {
			slog.Info("Email queued successfully", "recipients", to)
		}
	})

	return nil
}
