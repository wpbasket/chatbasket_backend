package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
)

// EmailRequest represents the incoming JSON structure.
//
// `Body`     - HTML body (required).
// `TextBody` - Optional plain-text alternative. If omitted, a plain-text
// version is automatically derived from the HTML body so every outgoing
// message is sent as a proper multipart/alternative (text + html). This
// is critical for inbox placement at Gmail / Outlook / Yahoo.
// `RefID`    - Optional opaque identifier echoed back via the
// `X-Entity-Ref-ID` header. Helps with tracking & dedup on the recipient
// side and lowers hash-similarity spam scoring.
type EmailRequest struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	TextBody string   `json:"text_body,omitempty"`
	RefID    string   `json:"ref_id,omitempty"`
}

// Config holds the relay configuration
type Config struct {
	Secret       string
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPass     string
	SMTPFrom     string
	SMTPFromName string
}

var (
	jobQueue = make(chan EmailRequest, 200) // Buffer for request spikes
	wg       sync.WaitGroup
)

func main() {
	// 1. Set Memory Limit for 512MB RAM safely (450MiB)
	// This helps Go's GC be more aggressive before hitting the dyno limit
	debug.SetMemoryLimit(450 * 1024 * 1024)

	config := loadConfig()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 2. Start Worker Pool (5 workers is safe for 512MB and respects Zoho)
	numWorkers := 5
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go emailWorker(ctx, i, config)
	}

	// 3. Setup HTTP Server
	mux := http.NewServeMux()
	mux.HandleFunc("/", makeHandler(config))
	mux.HandleFunc("/health", healthCheckHandler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// 4. Graceful Shutdown Handling
	// Go 1.26: Using signal.NotifyContext for better context-based shutdown
	go func() {
		shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		<-shutdownCtx.Done()

		// Log the signal that caused shutdown
		if cause := context.Cause(shutdownCtx); cause != nil {
			log.Printf("Shutting down relay due to: %v", cause)
		} else {
			log.Println("Shutting down relay...")
		}

		// Stop accepting new HTTP requests
		httpShutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(httpShutdownCtx)

		// Stop workers and wait for pending jobs
		cancel()
		wg.Wait()
		log.Println("Relay exited gracefully")
		os.Exit(0)
	}()

	log.Printf("Relay version 2.0 (Worker Pool) listening on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}

	// Wait for the shutdown goroutine to exit the process
	select {}
}

func loadConfig() Config {
	secret := os.Getenv("MAIL_RELAY_SECRET")
	if secret == "" {
		log.Fatal("MAIL_RELAY_SECRET environment variable must be set")
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")

	if smtpHost == "" || smtpPort == "" || smtpUser == "" || smtpPass == "" || smtpFrom == "" {
		log.Fatal("SMTP configuration incomplete: SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, and SMTP_FROM must all be set")
	}

	return Config{
		Secret:       secret,
		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUser:     smtpUser,
		SMTPPass:     smtpPass,
		SMTPFrom:     smtpFrom,
		SMTPFromName: os.Getenv("SMTP_FROM_NAME"),
	}
}

// sanitizeHeader removes \r and \n characters to prevent header injection
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// validateEmailRequest validates the email request payload
func validateEmailRequest(req EmailRequest) error {
	// Check To array is not empty
	if len(req.To) == 0 {
		return fmt.Errorf("to array cannot be empty")
	}

	// Validate each email address
	for _, addr := range req.To {
		if _, err := mail.ParseAddress(addr); err != nil {
			return fmt.Errorf("invalid email address: %s", addr)
		}
	}

	// Check length limits
	if len(req.Subject) > 998 { // RFC 2822 line length limit
		return fmt.Errorf("subject too long (max 998 characters)")
	}

	if len(req.Body) > 500000 { // 500KB limit for body
		return fmt.Errorf("body too long (max 500KB)")
	}

	// Limit number of recipients
	if len(req.To) > 50 {
		return fmt.Errorf("too many recipients (max 50)")
	}

	return nil
}

func makeHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Auth - use constant-time comparison
		providedSecret := r.Header.Get("X-Relay-Secret")
		if subtle.ConstantTimeCompare([]byte(cfg.Secret), []byte(providedSecret)) != 1 {
			log.Printf("Unauthorized access from %s - User-Agent: %s - Secret prefix: %s",
				r.RemoteAddr,
				r.UserAgent(),
				truncateSecret(providedSecret))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Limit request body size to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		// Parse
		var req EmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("Invalid JSON from %s: %v", r.RemoteAddr, err)
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		// Validate request
		if err := validateEmailRequest(req); err != nil {
			log.Printf("Validation failed from %s: %v", r.RemoteAddr, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Push to Queue
		select {
		case jobQueue <- req:
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, "Queued")
		default:
			queueDepth := len(jobQueue)
			log.Printf("ALERT: Queue full (%d/200), rejecting request from %s", queueDepth, r.RemoteAddr)
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Service busy", http.StatusServiceUnavailable)
		}
	}
}

// truncateSecret returns first 4 chars of secret for logging (or less if shorter)
func truncateSecret(s string) string {
	if len(s) > 4 {
		return s[:4] + "..."
	}
	if len(s) == 0 {
		return "(empty)"
	}
	return s + "..."
}

// healthCheckHandler provides a health check endpoint
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	queueDepth := len(jobQueue)
	if queueDepth > 180 { // 90% full
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"degraded","queue":%d,"capacity":200}`, queueDepth)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","queue":%d,"capacity":200}`, queueDepth)
}

func emailWorker(ctx context.Context, id int, cfg Config) {
	defer wg.Done()
	log.Printf("Worker %d started", id)

	addr := fmt.Sprintf("%s:%s", cfg.SMTPHost, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down...", id)
			return
		case req := <-jobQueue:
			// Only send to first recipient (as per original logic).
			// mail.ParseAddress already validated it in the HTTP handler,
			// but defend-in-depth: sanitize once more to strip CR/LF.
			toAddr := sanitizeHeader(req.To[0])

			msg, err := buildMIMEMessage(cfg, toAddr, req.Subject, req.Body, req.TextBody, req.RefID)
			if err != nil {
				log.Printf("Worker %d Build Error for %v: %v", id, toAddr, err)
				continue
			}

			if err := sendMail(addr, auth, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom, []string{toAddr}, msg); err != nil {
				log.Printf("Worker %d Error for %v: %v", id, toAddr, err)
			} else {
				log.Printf("Worker %d Success: Email sent to %v (Subject: %q)", id, toAddr, sanitizeHeader(req.Subject))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// MIME message builder
// ---------------------------------------------------------------------------

// buildMIMEMessage produces an RFC 5322 / 2045 compliant multipart/alternative
// message (text + html) ready to be passed to net/smtp. The exact headers and
// MIME layout are tuned for inbox placement at Gmail, Outlook and Yahoo:
//
//   - RFC 5322 mandatory headers: Date, From, To, Message-ID.
//   - Subject and From-display-name are RFC 2047 encoded.
//   - Body is multipart/alternative with both text/plain and text/html parts,
//     each quoted-printable encoded with UTF-8 charset.
//   - Auto-Submitted: auto-generated marks the mail as transactional, which
//     suppresses out-of-office replies and avoids list-loops.
//   - X-Entity-Ref-ID carries a per-message identifier to lower hash-similarity
//     spam scoring across many recipients receiving structurally similar mail.
//
// Note: List-Unsubscribe is intentionally NOT set. This relay is used for
// transactional security mail (OTPs, verification); users must not be able to
// "unsubscribe" from password resets. Gmail's bulk-sender unsubscribe rules
// apply only when sending bulk/promotional mail.
func buildMIMEMessage(cfg Config, to, subject, htmlBody, textBody, refID string) ([]byte, error) {
	// Sanitize inbound header content to guarantee no CR/LF injection.
	subject = sanitizeHeader(subject)
	fromName := sanitizeHeader(cfg.SMTPFromName)
	to = sanitizeHeader(to)

	// Parse the From address to ensure validity and to produce a properly
	// RFC 2047 / 5322 encoded From header (mail.Address.String handles this).
	fromAddr := mail.Address{Name: fromName, Address: cfg.SMTPFrom}

	// Derive plain-text alternative from HTML if the caller did not provide
	// one. This is non-negotiable for deliverability: HTML-only messages are
	// strongly penalised by Gmail and Outlook spam filters.
	if strings.TrimSpace(textBody) == "" {
		textBody = htmlToPlainText(htmlBody)
	}

	// Boundary must be unique per message and is restricted to a small,
	// MIME-safe charset. crypto/rand keeps it unguessable from the outside.
	boundary, err := randomBoundary()
	if err != nil {
		return nil, fmt.Errorf("generate boundary: %w", err)
	}

	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randomHex(8), domainOf(cfg.SMTPFrom))

	var buf bytes.Buffer
	buf.Grow(len(htmlBody) + len(textBody) + 1024)

	// --- Headers ---------------------------------------------------------
	writeHeader(&buf, "Date", time.Now().UTC().Format(time.RFC1123Z))
	writeHeader(&buf, "From", fromAddr.String())
	writeHeader(&buf, "To", to)
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("UTF-8", subject))
	writeHeader(&buf, "Message-ID", messageID)
	writeHeader(&buf, "MIME-Version", "1.0")
	writeHeader(&buf, "Auto-Submitted", "auto-generated")
	writeHeader(&buf, "X-Auto-Response-Suppress", "All")
	if refID != "" {
		writeHeader(&buf, "X-Entity-Ref-ID", sanitizeHeader(refID))
	} else {
		writeHeader(&buf, "X-Entity-Ref-ID", randomHex(16))
	}
	writeHeader(&buf, "Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	buf.WriteString("\r\n")

	// Preamble for non-MIME clients (very few left, but harmless and RFC2046).
	buf.WriteString("This is a multi-part message in MIME format.\r\n")

	// --- text/plain part -------------------------------------------------
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	if err := writeQuotedPrintable(&buf, textBody); err != nil {
		return nil, fmt.Errorf("encode text part: %w", err)
	}
	buf.WriteString("\r\n")

	// --- text/html part --------------------------------------------------
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	if err := writeQuotedPrintable(&buf, htmlBody); err != nil {
		return nil, fmt.Errorf("encode html part: %w", err)
	}
	buf.WriteString("\r\n")

	// Closing boundary.
	buf.WriteString("--" + boundary + "--\r\n")

	return buf.Bytes(), nil
}

// writeHeader appends a single header line. Values are assumed sanitized.
func writeHeader(buf *bytes.Buffer, name, value string) {
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

// writeQuotedPrintable encodes s as quoted-printable into buf.
func writeQuotedPrintable(buf *bytes.Buffer, s string) error {
	w := quotedprintable.NewWriter(buf)
	if _, err := w.Write([]byte(s)); err != nil {
		return err
	}
	return w.Close()
}

// randomBoundary returns a MIME boundary built from 16 random bytes.
func randomBoundary() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "----=_Part_" + hex.EncodeToString(b[:]), nil
}

// randomHex returns n bytes of crypto-random data, hex-encoded.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based pseudo-random — collisions here are
		// not a security issue, only a uniqueness concern.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// domainOf extracts the domain part of an email address. Used for Message-ID.
func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return "localhost"
}

// Pre-compiled regexes used by htmlToPlainText. Compiling once at package
// init keeps the email build path allocation-free in the hot loop.
var (
	// Go's RE2 engine has no backreferences, so we match script and style
	// blocks with two separate alternations rather than `\1`.
	reScriptStyle = regexp.MustCompile(`(?is)<script[^>]*>.*?</\s*script\s*>|<style[^>]*>.*?</\s*style\s*>`)
	reBlockBreak  = regexp.MustCompile(`(?i)</?(p|div|br|li|tr|h[1-6]|hr)[^>]*>`)
	reTag         = regexp.MustCompile(`<[^>]+>`)
	reMultiSpace  = regexp.MustCompile(`[ \t]+`)
	reMultiBlank  = regexp.MustCompile(`\n{3,}`)
)

// htmlToPlainText produces a readable plain-text rendering of an HTML body.
// It is intentionally simple — the goal is a sane fallback for clients that
// prefer plain text and for spam filters that demand a text alternative,
// not pixel-perfect rendering.
func htmlToPlainText(s string) string {
	// Strip script/style content entirely.
	s = reScriptStyle.ReplaceAllString(s, "")
	// Convert block-level tags into newlines so structure survives.
	s = reBlockBreak.ReplaceAllString(s, "\n")
	// Drop remaining tags.
	s = reTag.ReplaceAllString(s, "")
	// Decode HTML entities (&amp;, &nbsp;, &#39; ...).
	s = html.UnescapeString(s)
	// Normalise whitespace.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = reMultiBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// SMTP transport (STARTTLS on 587, implicit TLS on 465)
// ---------------------------------------------------------------------------

// sendMail wraps net/smtp.SendMail with explicit handling of implicit TLS
// (SMTPS on port 465) which the stdlib helper does not support. On port 587
// (or any other plain port that advertises STARTTLS) the standard helper is
// used, which already negotiates STARTTLS before authenticating.
func sendMail(addr string, auth smtp.Auth, host, port, from string, to []string, msg []byte) error {
	if port == "465" {
		return sendMailImplicitTLS(addr, auth, host, from, to, msg)
	}
	// 587 / 25 path. Go's smtp.SendMail performs STARTTLS automatically
	// when the server advertises it, and refuses PLAIN auth over an
	// unencrypted channel — exactly what we want.
	return smtp.SendMail(addr, auth, from, to, msg)
}

// sendMailImplicitTLS dials an SMTPS server (TLS from the first byte).
func sendMailImplicitTLS(addr string, auth smtp.Auth, host, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("dial tls: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("auth: %w", err)
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt to %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return client.Quit()
}
