package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
)

// EmailRequest represents the incoming JSON structure
type EmailRequest struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
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
			// Sanitize all header fields to prevent injection
			sanitizedSubject := sanitizeHeader(req.Subject)
			sanitizedFromName := sanitizeHeader(cfg.SMTPFromName)

			// Only send to first recipient (as per original logic)
			// Sanitize the To address as well
			sanitizedTo := sanitizeHeader(req.To[0])

			msg := fmt.Sprintf("From: %s <%s>\r\n"+
				"To: %s\r\n"+
				"Subject: %s\r\n"+
				"MIME-version: 1.0;\r\n"+
				"Content-Type: text/html; charset=\"UTF-8\";\r\n"+
				"\r\n"+
				"%s\r\n", sanitizedFromName, cfg.SMTPFrom, sanitizedTo, sanitizedSubject, req.Body)

			err := smtp.SendMail(addr, auth, cfg.SMTPFrom, []string{req.To[0]}, []byte(msg))
			if err != nil {
				log.Printf("Worker %d Error for %v: %v", id, req.To[0], err)
			} else {
				log.Printf("Worker %d Success: Email sent to %v (Subject: %s)", id, req.To[0], sanitizedSubject)
			}
		}
	}
}
