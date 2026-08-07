package clients

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatbasket-api/internal/platform/config"

	rpc_core_emailv1 "chatbasket-api/gen/proto/core/core_email"
	rpc_core_emailv1connect "chatbasket-api/gen/proto/core/core_email/rpc_core_emailv1connect"

	"connectrpc.com/connect"
)

const (
	relaySignatureHeader = "X-Relay-Signature"
	relayTimestampHeader = "X-Relay-Timestamp"
	relayNonceHeader     = "X-Relay-Nonce"

	// relaySignatureVersion prefixes the signature header so the scheme can
	// be rotated later without ambiguity. The gateway accepts this one only.
	relaySignatureVersion = "v1"

	// emailRequestTimeout bounds a single call to the gateway.
	emailRequestTimeout = 30 * time.Second
)

var emailWG sync.WaitGroup

var (
	// emailTransport is the single connection pool used for every message.
	// Reusing it keeps TCP connections and TLS sessions warm between emails
	// instead of paying for a fresh handshake per message.
	emailTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   8,
		MaxConnsPerHost:       16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS13},
	}

	// emailClient talks to the gateway over Connect RPC. It stays nil when
	// the relay is not configured, in which case sends are skipped.
	emailClient rpc_core_emailv1connect.EmailServiceClient
)

// InitEmailClient wires up the Connect RPC client for the Heroku mail relay.
// Call it once during startup. When the relay is not configured, or its URL is
// not one we are willing to send a signed request to, the client is left unset
// and every send becomes a logged no-op, so the app still boots.
func InitEmailClient(cfg *config.EmailConfig) {
	if cfg == nil || cfg.RelayURL == "" || cfg.RelaySecret == "" {
		relayURL := ""
		if cfg != nil {
			relayURL = cfg.RelayURL
		}
		slog.Warn("Mail relay configuration is missing", "url", relayURL)
		return
	}

	baseURL, err := relayBaseURL(cfg.RelayURL)
	if err != nil {
		slog.Error("Mail relay URL was rejected, email is disabled", "url", cfg.RelayURL, "error", err)
		return
	}

	emailClient = rpc_core_emailv1connect.NewEmailServiceClient(
		&http.Client{
			Timeout:   emailRequestTimeout,
			Transport: &relaySigner{base: emailTransport, secret: []byte(cfg.RelaySecret)},
		},
		baseURL,
		connect.WithSendGzip(),
		// OTP payloads are a few hundred bytes; compressing those costs more
		// than it saves. Only the large HTML bodies get gzipped.
		connect.WithCompressMinBytes(1024),
		// The gateway only ever answers with a three field acknowledgement.
		connect.WithReadMaxBytes(64<<10),
	)
}

// relayBaseURL validates MAIL_RELAY_URL and returns the bare origin the
// generated client appends the procedure path to. Plain HTTP is refused
// outside loopback so a request can never leave the host in clear text.
func relayBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("not a URL: %w", err)
	}
	if parsed.Hostname() == "" {
		return "", errors.New("missing host")
	}
	if strings.Trim(parsed.Path, "/") != "" {
		return "", fmt.Errorf("must be an origin without a path, got %q", parsed.Path)
	}

	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return "", errors.New("plain http is only allowed for loopback addresses")
		}
	default:
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}

	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// relaySigner signs every outbound request with HMAC-SHA256, so the gateway
// can authenticate it without the shared secret ever crossing the wire and can
// reject anything that was replayed or altered in transit.
type relaySigner struct {
	base   http.RoundTripper
	secret []byte
}

func (s *relaySigner) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := readRequestBody(req)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate relay nonce: %w", err)
	}
	encodedNonce := hex.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// RoundTrip must not mutate the request it was handed.
	signed := req.Clone(req.Context())
	signed.Body = io.NopCloser(bytes.NewReader(body))
	signed.ContentLength = int64(len(body))
	signed.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	signed.Header.Set(relayTimestampHeader, timestamp)
	signed.Header.Set(relayNonceHeader, encodedNonce)
	signed.Header.Set(relaySignatureHeader, relaySignatureVersion+"="+
		signRelayRequest(s.secret, timestamp, encodedNonce, signed.URL.Path, body))

	return s.base.RoundTrip(signed)
}

// readRequestBody buffers the outgoing body so it can be both signed and sent.
func readRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read relay request body: %w", err)
	}
	return body, nil
}

// signRelayRequest returns the hex encoded HMAC-SHA256 of the canonical
// request string. The gateway rebuilds the same string in app/security.go;
// the two implementations must stay identical.
func signRelayRequest(secret []byte, timestamp, nonce, path string, body []byte) string {
	digest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.Join([]string{
		relaySignatureVersion,
		timestamp,
		nonce,
		path,
		hex.EncodeToString(digest[:]),
	}, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
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
	if emailClient == nil {
		slog.Warn("Mail relay client is not initialized, skipping email", "recipients", to)
		return nil // Return nil so flow continues, but log the error
	}

	// Fire and forget in a goroutine using Go 1.26 WaitGroup.Go()
	emailWG.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered from panic in email goroutine", "error", r)
			}
		}()

		// Use a 30s timeout for the background relay call
		ctx, cancel := context.WithTimeout(context.Background(), emailRequestTimeout)
		defer cancel()

		_, err := emailClient.SendEmail(ctx, connect.NewRequest(&rpc_core_emailv1.SendEmailRequest{
			To:       to,
			Subject:  subject,
			Body:     bodyHTML,
			TextBody: bodyText,
			RefId:    refID,
		}))
		if err != nil {
			slog.Error("Email relay call failed", "error", err, "code", connect.CodeOf(err).String())
			return
		}

		slog.Info("Email queued successfully", "recipients", to)
	})

	return nil
}
