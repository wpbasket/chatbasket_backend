package utils

import (
	"bytes"
	"chatbasket-api/model"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// SendEmail sends an email using SMTP with STARTTLS (port 587)
func SendEmail(to []string, subject string, bodyHTML string) *model.AppError {
	relayURL := os.Getenv("MAIL_RELAY_URL")
	relaySecret := os.Getenv("MAIL_RELAY_SECRET")

	if relayURL == "" || relaySecret == "" {
		return &model.AppError{Type: "config_error", Message: "Mail relay configuration is missing"}
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"to":      to,
		"subject": subject,
		"body":    bodyHTML,
	})

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", relayURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return &model.AppError{Type: "internal_error", Message: "Failed to create relay request: " + err.Error()}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Relay-Secret", relaySecret)

	resp, err := client.Do(req)
	if err != nil {
		return &model.AppError{Type: "network_error", Message: "Relay connection failed: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &model.AppError{Type: "relay_error", Message: fmt.Sprintf("Relay returned status %d", resp.StatusCode)}
	}

	return nil
}
