package utils

import (
	"chatbasket-api/model"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
)

// SendEmail sends an email using SMTP with STARTTLS (port 587)
func SendEmail(to []string, subject string, bodyHTML string) *model.AppError {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	fromName := os.Getenv("SMTP_FROM_NAME")

	if host == "" || port == "" || username == "" || password == "" {
		return &model.AppError{Type: "config_error", Message: "SMTP configuration is missing"}
	}

	// Build From header with display name if provided
	fromHeader := from
	if fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, from)
	}

	// Build message headers
	headers := make(map[string]string)
	headers["From"] = fromHeader
	headers["To"] = to[0]
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"UTF-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + bodyHTML

	// Connect to SMTP server
	addr := host + ":" + port
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return &model.AppError{Type: "email_send_error", Message: "dial failed: " + err.Error()}
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return &model.AppError{Type: "email_send_error", Message: "SMTP client creation failed: " + err.Error()}
	}
	defer client.Close()

	// Upgrade to TLS (STARTTLS)
	tlsConfig := &tls.Config{
		ServerName: host,
	}
	if err = client.StartTLS(tlsConfig); err != nil {
		return &model.AppError{Type: "email_send_error", Message: "STARTTLS failed: " + err.Error()}
	}

	// Authenticate
	auth := smtp.PlainAuth("", username, password, host)
	if err = client.Auth(auth); err != nil {
		return &model.AppError{Type: "email_send_error", Message: "SMTP auth failed: " + err.Error()}
	}

	// Set sender and recipients
	if err = client.Mail(from); err != nil {
		return &model.AppError{Type: "email_send_error", Message: "MAIL FROM failed: " + err.Error()}
	}
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return &model.AppError{Type: "email_send_error", Message: "RCPT TO failed: " + err.Error()}
		}
	}

	// Send message body
	writer, err := client.Data()
	if err != nil {
		return &model.AppError{Type: "email_send_error", Message: "DATA failed: " + err.Error()}
	}
	_, err = writer.Write([]byte(message))
	if err != nil {
		return &model.AppError{Type: "email_send_error", Message: "message write failed: " + err.Error()}
	}
	err = writer.Close()
	if err != nil {
		return &model.AppError{Type: "email_send_error", Message: "message close failed: " + err.Error()}
	}

	return nil
}
