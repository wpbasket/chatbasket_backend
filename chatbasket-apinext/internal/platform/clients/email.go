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

// EmailRelayPayload represents the payload sent to the email relay service, ported from utils/emailUtils.go
type EmailRelayPayload struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

// WaitEmails waits for all background email tasks to complete, ported from utils/emailUtils.go
func WaitEmails() {
	emailWG.Wait()
}

// SendEmail sends an email using the Heroku Relay in a "fire and forget" background task, ported from utils/emailUtils.go
func SendEmail(to []string, subject string, bodyHTML string) error {
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
			To:      to,
			Subject: subject,
			Body:    bodyHTML,
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
