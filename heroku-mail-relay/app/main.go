package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"runtime/debug"
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

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// 4. Graceful Shutdown Handling
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop

		log.Println("Shutting down relay...")

		// Stop accepting new HTTP requests
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)

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
	return Config{
		Secret:       os.Getenv("MAIL_RELAY_SECRET"),
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     os.Getenv("SMTP_PORT"),
		SMTPUser:     os.Getenv("SMTP_USERNAME"),
		SMTPPass:     os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		SMTPFromName: os.Getenv("SMTP_FROM_NAME"),
	}
}

func makeHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Auth
		providedSecret := r.Header.Get("X-Relay-Secret")
		if cfg.Secret == "" || cfg.Secret != providedSecret {
			log.Printf("Unauthorized access from %s", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse
		var req EmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		// Push to Queue
		select {
		case jobQueue <- req:
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, "Queued")
		default:
			log.Println("Error: Job queue is full! Under heavy load.")
			http.Error(w, "Service busy", http.StatusServiceUnavailable)
		}
	}
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
			msg := fmt.Sprintf("From: %s <%s>\r\n"+
				"To: %s\r\n"+
				"Subject: %s\r\n"+
				"MIME-version: 1.0;\r\n"+
				"Content-Type: text/html; charset=\"UTF-8\";\r\n"+
				"\r\n"+
				"%s\r\n", cfg.SMTPFromName, cfg.SMTPFrom, req.To[0], req.Subject, req.Body)

			err := smtp.SendMail(addr, auth, cfg.SMTPFrom, req.To, []byte(msg))
			if err != nil {
				log.Printf("Worker %d Error for %v: %v", id, req.To, err)
			} else {
				log.Printf("Worker %d Success: Email sent to %v", id, req.To)
			}
		}
	}
}
