package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Request authentication
//
// Callers are authenticated with an HMAC-SHA256 signature rather than a bare
// shared secret: the secret never travels on the wire, the signature covers
// the exact bytes that were sent, and a captured request cannot be replayed.
// Everything here runs as plain HTTP middleware in front of the Connect
// handler, so an unauthenticated caller is rejected before a single byte of
// its body is read, decompressed or decoded.
//
// The canonical string both sides sign is:
//
//	v1 \n <unix-timestamp> \n <nonce> \n <request-path> \n <hex sha256 of body>
//
// The backend produces it in internal/platform/clients/email.go; the two
// implementations must stay identical.
// ---------------------------------------------------------------------------

const (
	relaySignatureHeader = "X-Relay-Signature"
	relayTimestampHeader = "X-Relay-Timestamp"
	relayNonceHeader     = "X-Relay-Nonce"

	// relaySignatureVersion prefixes the signature header so the scheme can
	// be rotated later without ambiguity.
	relaySignatureVersion = "v1"

	// maxClockSkew is how far a request timestamp may sit from the relay
	// clock. It doubles as the window a nonce has to be remembered for.
	maxClockSkew = 60 * time.Second

	// maxRequestBytes caps the bytes accepted from the wire, before Connect
	// decompresses and decodes them.
	maxRequestBytes = 1 << 20

	// expectedContentType is the only media type the gateway serves: binary
	// protobuf over Connect's unary protocol. JSON is refused.
	expectedContentType = "application/proto"

	// maxAuthFailures is how many rejected requests one client address may
	// produce inside authFailureWindow before it is throttled outright.
	// Stale-but-validly-signed rejections are exempt - see errStaleTimestamp.
	maxAuthFailures   = 10
	authFailureWindow = time.Minute

	// maxTrackedClients bounds the throttle table so a distributed flood
	// cannot grow it without limit.
	maxTrackedClients = 1024

	// maxInFlight bounds concurrent RPCs so a burst cannot exhaust the
	// 512MB dyno while the workers drain the queue.
	maxInFlight = 32

	// maxNonces bounds the replay cache. Only correctly signed requests
	// ever reach it, so an outsider cannot push it to its ceiling.
	maxNonces = 4096
)

// signRelayRequest returns the hex encoded HMAC-SHA256 of the canonical
// request string.
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

// relayGuard authenticates, filters and throttles every RPC before Connect is
// allowed to touch the request body.
func relayGuard(cfg Config, next http.Handler) http.Handler {
	secret := []byte(cfg.Secret)
	nonces := newNonceCache()
	failures := newFailureLimiter()
	inFlight := make(chan struct{}, maxInFlight)

	reject := func(w http.ResponseWriter, r *http.Request, addr string, now time.Time, status int, code, reason string) {
		failures.record(addr, now)
		log.Printf("Rejected %s %s from %s: %s (User-Agent: %q)",
			r.Method, r.URL.Path, addr, reason, r.Header.Get("User-Agent"))
		writeConnectError(w, status, code, reason)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case inFlight <- struct{}{}:
			defer func() { <-inFlight }()
		default:
			w.Header().Set("Retry-After", "5")
			writeConnectError(w, http.StatusTooManyRequests, "resource_exhausted", "too many concurrent requests")
			return
		}

		addr := clientAddr(r)
		now := time.Now()

		if failures.blocked(addr, now) {
			w.Header().Set("Retry-After", "60")
			writeConnectError(w, http.StatusTooManyRequests, "resource_exhausted", "too many failed attempts")
			return
		}

		// Heroku's router sets X-Forwarded-Proto. Its absence means nobody
		// proxied us (local run); its presence with anything but https means
		// the request reached the router in clear text.
		if proto := lastForwardedValue(r.Header.Get("X-Forwarded-Proto")); proto != "" && !strings.EqualFold(proto, "https") {
			reject(w, r, addr, now, http.StatusForbidden, "permission_denied", "https is required")
			return
		}

		if !addrAllowed(cfg.AllowedIPs, addr) {
			reject(w, r, addr, now, http.StatusForbidden, "permission_denied", "source address is not allowed")
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			reject(w, r, addr, now, http.StatusMethodNotAllowed, "unimplemented", "only POST is supported")
			return
		}

		if !hasContentType(r, expectedContentType) {
			reject(w, r, addr, now, http.StatusUnsupportedMediaType, "invalid_argument", "content type must be "+expectedContentType)
			return
		}

		if r.ContentLength > maxRequestBytes {
			reject(w, r, addr, now, http.StatusRequestEntityTooLarge, "resource_exhausted", "request body is too large")
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err != nil {
			reject(w, r, addr, now, http.StatusRequestEntityTooLarge, "resource_exhausted", "request body is too large")
			return
		}

		if err := verifyRelaySignature(secret, r, body, now, nonces); err != nil {
			if errors.Is(err, errStaleTimestamp) {
				// The signature is genuine, so this is the key holder with a
				// drifted clock, not someone probing the secret. Rejecting
				// without spending the failure budget keeps clock drift from
				// locking the backend out of its own gateway.
				log.Printf("Rejected %s %s from %s: %s - the signature is valid, check clock synchronization (User-Agent: %q)",
					r.Method, r.URL.Path, addr, err, r.Header.Get("User-Agent"))
				writeConnectError(w, http.StatusUnauthorized, "unauthenticated", err.Error())
				return
			}
			reject(w, r, addr, now, http.StatusUnauthorized, "unauthenticated", err.Error())
			return
		}

		// Hand the buffered body to Connect untouched.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		// A malformed protobuf never reaches the handler - Connect's codec
		// rejects it first - so log every non success here to keep that
		// visible.
		if recorder.status >= http.StatusBadRequest {
			log.Printf("Request from %s to %s failed with status %d", addr, r.URL.Path, recorder.status)
		}
	})
}

// errStaleTimestamp rejects a request whose signature is valid but whose
// timestamp sits outside the replay window. It is the one rejection that
// proves the caller holds the key, so relayGuard refuses it without counting
// it as a failure: a drifted clock keeps failing loudly, request by request,
// instead of cutting the address off together with its valid traffic.
var errStaleTimestamp = errors.New("stale timestamp")

// verifyRelaySignature checks that the request carries a fresh, untampered and
// unused signature produced with the shared secret.
func verifyRelaySignature(secret []byte, r *http.Request, body []byte, now time.Time, nonces *nonceCache) error {
	provided := r.Header.Get(relaySignatureHeader)
	timestamp := r.Header.Get(relayTimestampHeader)
	nonce := r.Header.Get(relayNonceHeader)
	if provided == "" || timestamp == "" || nonce == "" {
		return errors.New("missing signature headers")
	}
	if len(nonce) < 32 || len(nonce) > 128 {
		return errors.New("malformed nonce")
	}

	version, signature, ok := strings.Cut(provided, "=")
	if !ok || version != relaySignatureVersion {
		return errors.New("unsupported signature version")
	}

	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("malformed timestamp")
	}

	expected := signRelayRequest(secret, timestamp, nonce, r.URL.Path, body)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("signature mismatch")
	}

	// Freshness is checked only after the signature, so a stale rejection is
	// known to come from the key holder. A stale probe without the key still
	// spends its failure budget - it fails above as a mismatch.
	if skew := now.Sub(time.Unix(seconds, 0)); skew > maxClockSkew || skew < -maxClockSkew {
		return errStaleTimestamp
	}

	if !nonces.accept(nonce, now) {
		return errors.New("replayed request")
	}
	return nil
}

// writeConnectError answers with the JSON error envelope of the Connect
// protocol so the caller reports a real Connect code instead of a bare status.
func writeConnectError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
}

// clientAddr returns the address Heroku's router observed. The router appends
// the connecting peer as the last X-Forwarded-For hop, so whatever a caller
// puts in that header itself is ignored.
func clientAddr(r *http.Request) string {
	if last := lastForwardedValue(r.Header.Get("X-Forwarded-For")); last != "" {
		return last
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// lastForwardedValue returns the final entry of a comma separated proxy
// header. The router's own observation is the one appended last, so anything
// a caller put in front of it carries no weight.
func lastForwardedValue(raw string) string {
	if raw == "" {
		return ""
	}
	hops := strings.Split(raw, ",")
	return strings.TrimSpace(hops[len(hops)-1])
}

// addrAllowed reports whether addr is covered by the configured allowlist. An
// empty allowlist accepts everyone, which is the default.
func addrAllowed(allowed []*net.IPNet, addr string) bool {
	if len(allowed) == 0 {
		return true
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, network := range allowed {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// parseAllowedIPs turns MAIL_RELAY_ALLOWED_IPS into networks. Bare addresses
// are treated as single host entries; an empty value disables the allowlist.
func parseAllowedIPs(raw string) ([]*net.IPNet, error) {
	var networks []*net.IPNet
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", entry, err)
			}
			networks = append(networks, network)
			continue
		}

		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("invalid address %q", entry)
		}
		if v4 := ip.To4(); v4 != nil {
			networks = append(networks, &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)})
		} else {
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	return networks, nil
}

// hasContentType reports whether the request media type equals want.
func hasContentType(r *http.Request, want string) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, want)
}

// statusRecorder remembers the status code written downstream.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// nonceCache remembers accepted nonces for the length of the clock skew
// window, so a signed request can be used exactly once.
type nonceCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newNonceCache() *nonceCache {
	return &nonceCache{seen: make(map[string]time.Time)}
}

// accept records nonce and reports whether it had not been seen before.
func (c *nonceCache) accept(nonce string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, expiry := range c.seen {
		if now.After(expiry) {
			delete(c.seen, key)
		}
	}
	if _, replayed := c.seen[nonce]; replayed {
		return false
	}
	if len(c.seen) >= maxNonces {
		return false
	}

	c.seen[nonce] = now.Add(maxClockSkew)
	return true
}

// failureLimiter throttles a client address that keeps failing the guard, so
// the signing key cannot be probed at speed.
type failureLimiter struct {
	mu      sync.Mutex
	entries map[string]*failureEntry
}

type failureEntry struct {
	count   int
	resetAt time.Time
}

func newFailureLimiter() *failureLimiter {
	return &failureLimiter{entries: make(map[string]*failureEntry)}
}

// blocked reports whether addr has spent its failure budget for this window.
func (l *failureLimiter) blocked(addr string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)
	entry, tracked := l.entries[addr]
	return tracked && entry.count >= maxAuthFailures
}

// record counts one rejected request against addr.
func (l *failureLimiter) record(addr string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)
	if entry, tracked := l.entries[addr]; tracked {
		entry.count++
		return
	}
	if len(l.entries) >= maxTrackedClients {
		return
	}
	l.entries[addr] = &failureEntry{count: 1, resetAt: now.Add(authFailureWindow)}
}

func (l *failureLimiter) prune(now time.Time) {
	for addr, entry := range l.entries {
		if now.After(entry.resetAt) {
			delete(l.entries, addr)
		}
	}
}
