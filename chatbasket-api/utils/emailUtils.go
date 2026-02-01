package utils

import (
	"bytes"
	"chatbasket-api/model"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

var emailWG sync.WaitGroup

// WaitEmails waits for all background email tasks to complete.
func WaitEmails() {
	emailWG.Wait()
}

// SendEmail sends an email using the Heroku Relay in a "fire and forget" background task
func SendEmail(to []string, subject string, bodyHTML string) *model.AppError {
	relayURL := os.Getenv("MAIL_RELAY_URL")
	relaySecret := os.Getenv("MAIL_RELAY_SECRET")

	if relayURL == "" || relaySecret == "" {
		fmt.Printf("Email Warning: Mail relay configuration is missing. URL: %s\n", relayURL)
		return nil // Return nil so flow continues, but log the error
	}

	// Fire and forget in a goroutine
	emailWG.Add(1)
	go func() {
		defer emailWG.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Relay Critical: Recovered from panic in email goroutine: %v\n", r)
			}
		}()

		reqBody, _ := json.Marshal(map[string]interface{}{
			"to":      to,
			"subject": subject,
			"body":    bodyHTML,
		})

		// Use a 30s timeout for the background relay call
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequest("POST", relayURL, bytes.NewBuffer(reqBody))
		if err != nil {
			fmt.Printf("Relay Error: Failed to create request: %v\n", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Relay-Secret", relaySecret)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Relay Error: Connection failed: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			fmt.Printf("Relay Error: Server returned status %d\n", resp.StatusCode)
		} else {
			fmt.Printf("Relay Success (Accepted): Email queued for %v\n", to)
		}
	}()

	return nil
}
