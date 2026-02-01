package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
)

type EmailRequest struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/send", handleSendEmail)

	log.Printf("Relay listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleSendEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth Check
	secret := os.Getenv("MAIL_RELAY_SECRET")
	providedSecret := r.Header.Get("X-Relay-Secret")
	if secret == "" || secret != providedSecret {
		log.Printf("Unauthorized access attempt from %s", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req EmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// SMTP Config
	host := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	fromName := os.Getenv("SMTP_FROM_NAME")

	if host == "" || smtpPort == "" || username == "" || password == "" {
		http.Error(w, "Relay misconfigured", http.StatusInternalServerError)
		return
	}

	// Prepare Email
	addr := fmt.Sprintf("%s:%s", host, smtpPort)
	auth := smtp.PlainAuth("", username, password, host)
	
	msg := fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-version: 1.0;\r\n"+
		"Content-Type: text/html; charset=\"UTF-8\";\r\n"+
		"\r\n"+
		"%s\r\n", fromName, from, req.To[0], req.Subject, req.Body)

	// Send
	err := smtp.SendMail(addr, auth, from, req.To, []byte(msg))
	if err != nil {
		log.Printf("Failed to send email: %v", err)
		http.Error(w, fmt.Sprintf("Failed to send email: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully relayed email to %v", req.To)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Email sent successfully")
}
