package clients

import (
	"bytes"
	appconfig "chatbasket-api/internal/platform/config"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	rpc_core_emailv1 "chatbasket-api/gen/proto/core/core_email"
	rpc_core_emailv1connect "chatbasket-api/gen/proto/core/core_email/rpc_core_emailv1connect"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRelaySecret = "test-relay-secret"

func TestEmailTransport_AllowsTLS12(t *testing.T) {
	require.NotNil(t, emailTransport.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), emailTransport.TLSClientConfig.MinVersion)
}

// recordingEmailHandler stands in for the Heroku mail gateway and captures
// everything the backend puts on the wire.
type recordingEmailHandler struct {
	mu       sync.Mutex
	requests []*rpc_core_emailv1.SendEmailRequest
	headers  []http.Header

	// err, when set, is returned instead of a success response.
	err error
	// release, when set, blocks the handler until the channel is closed.
	release chan struct{}
}

func (h *recordingEmailHandler) SendEmail(
	_ context.Context,
	req *connect.Request[rpc_core_emailv1.SendEmailRequest],
) (*connect.Response[rpc_core_emailv1.SendEmailResponse], error) {
	if h.release != nil {
		<-h.release
	}

	h.mu.Lock()
	h.requests = append(h.requests, req.Msg)
	h.headers = append(h.headers, req.Header().Clone())
	h.mu.Unlock()

	if h.err != nil {
		return nil, h.err
	}

	return connect.NewResponse(&rpc_core_emailv1.SendEmailResponse{
		Queued:        true,
		QueueDepth:    1,
		QueueCapacity: 200,
	}), nil
}

func (h *recordingEmailHandler) received() []*rpc_core_emailv1.SendEmailRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.requests
}

// verifyingRelay mirrors the gateway's guard: it recomputes the HMAC over the
// exact bytes it received and refuses anything that does not match. The two
// signing implementations live in separate Go modules, so this is what proves
// they agree.
func verifyingRelay(t *testing.T, next http.Handler) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the relay request body failed: %v", err)
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}

		timestamp := r.Header.Get(relayTimestampHeader)
		nonce := r.Header.Get(relayNonceHeader)
		expected := relaySignatureVersion + "=" +
			signRelayRequest([]byte(testRelaySecret), timestamp, nonce, r.URL.Path, body)

		if got := r.Header.Get(relaySignatureHeader); got != expected {
			t.Errorf("signature = %q, want %q", got, expected)
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		next.ServeHTTP(w, r)
	})
}

// newRelayStub starts an in-process gateway serving the generated handler
// behind the same signature check the real gateway applies.
func newRelayStub(t *testing.T, handler *recordingEmailHandler) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	path, h := rpc_core_emailv1connect.NewEmailServiceHandler(handler)
	mux.Handle(path, verifyingRelay(t, h))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// initTestEmailClient points the package level client at the stub and resets
// it once the test finishes.
func initTestEmailClient(t *testing.T, relayURL string) {
	t.Helper()

	t.Cleanup(func() { emailClient = nil })
	InitEmailClient(&appconfig.EmailConfig{RelayURL: relayURL, RelaySecret: testRelaySecret})
	require.NotNil(t, emailClient, "expected the relay client to be initialized")
}

func TestSendEmailExt_PayloadFidelity(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	err := SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code: 123456</p>", "Code: 123456", "otp-login")
	require.NoError(t, err)
	WaitEmails()

	received := handler.received()
	require.Len(t, received, 1)
	assert.Equal(t, []string{"user@example.com"}, received[0].GetTo())
	assert.Equal(t, "Your OTP", received[0].GetSubject())
	assert.Equal(t, "<p>Code: 123456</p>", received[0].GetBody())
	assert.Equal(t, "Code: 123456", received[0].GetTextBody())
	assert.Equal(t, "otp-login", received[0].GetRefId())
}

// TestSendEmailExt_SignsRequests checks the credentials on the wire: a fresh
// timestamp, a single-use nonce, and no shared secret anywhere.
func TestSendEmailExt_SignsRequests(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
	WaitEmails()
	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
	WaitEmails()

	require.Len(t, handler.headers, 2)
	for _, header := range handler.headers {
		assert.Empty(t, header.Get("X-Relay-Secret"), "the shared secret must never leave the process")
		assert.True(t, strings.HasPrefix(header.Get(relaySignatureHeader), relaySignatureVersion+"="))
		assert.Len(t, header.Get(relayNonceHeader), 32, "the nonce must be 16 random bytes, hex encoded")

		seconds, err := strconv.ParseInt(header.Get(relayTimestampHeader), 10, 64)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now(), time.Unix(seconds, 0), 30*time.Second)
	}

	assert.NotEqual(t, handler.headers[0].Get(relayNonceHeader), handler.headers[1].Get(relayNonceHeader),
		"every request needs its own nonce or the gateway would treat the second as a replay")
	assert.NotEqual(t, handler.headers[0].Get(relaySignatureHeader), handler.headers[1].Get(relaySignatureHeader))
}

// TestSendEmailExt_CompressesOnlyLargeBodies documents the wire optimisation:
// an OTP is too small to be worth gzipping, a real HTML body is not.
func TestSendEmailExt_CompressesOnlyLargeBodies(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code: 123456</p>", "", ""))
	WaitEmails()
	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Newsletter", strings.Repeat("<p>hello</p>", 500), "", ""))
	WaitEmails()

	require.Len(t, handler.headers, 2)
	assert.Empty(t, handler.headers[0].Get("Content-Encoding"), "a small payload must not pay for gzip")
	assert.Equal(t, "gzip", handler.headers[1].Get("Content-Encoding"), "a large payload must be gzipped")
}

func TestSendEmail_HTMLOnly(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	require.NoError(t, SendEmail([]string{"user@example.com"}, "Welcome", "<p>Hi</p>"))
	WaitEmails()

	received := handler.received()
	require.Len(t, received, 1)
	assert.Equal(t, "<p>Hi</p>", received[0].GetBody())
	assert.Empty(t, received[0].GetTextBody())
	assert.Empty(t, received[0].GetRefId())
}

// TestSendEmailExt_FireAndForget locks in the contract the core_auth OTP flows
// rely on: the call returns before the gateway has answered.
func TestSendEmailExt_FireAndForget(t *testing.T) {
	handler := &recordingEmailHandler{release: make(chan struct{})}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
	assert.Empty(t, handler.received(), "the gateway must not have been awaited")

	close(handler.release)
	WaitEmails()
	assert.Len(t, handler.received(), 1)
}

// TestSendEmailExt_SwallowsRelayFailure keeps the auth flows unaffected by a
// rejecting gateway: the failure is logged, never returned.
func TestSendEmailExt_SwallowsRelayFailure(t *testing.T) {
	handler := &recordingEmailHandler{
		err: connect.NewError(connect.CodeResourceExhausted, assert.AnError),
	}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL)

	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", "otp-login"))
	WaitEmails()

	assert.Len(t, handler.received(), 1)
}

func TestSendEmailExt_WithoutConfiguredRelay(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)

	cases := map[string]*appconfig.EmailConfig{
		"nil config":     nil,
		"empty config":   {},
		"missing url":    {RelaySecret: testRelaySecret},
		"missing secret": {RelayURL: srv.URL},
	}

	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			emailClient = nil
			t.Cleanup(func() { emailClient = nil })

			InitEmailClient(cfg)
			assert.Nil(t, emailClient, "an unconfigured relay must leave the client unset")

			require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
			WaitEmails()
			assert.Empty(t, handler.received())
		})
	}
}

// TestInitEmailClient_RejectsUnsafeRelayURL refuses to sign a request into a
// URL that would leak it, or that would not resolve to the procedure path.
func TestInitEmailClient_RejectsUnsafeRelayURL(t *testing.T) {
	cases := map[string]string{
		"plain http off host": "http://relay.example.com",
		"path segment":        "https://relay.example.com/relay",
		"unsupported scheme":  "ftp://relay.example.com",
		"missing host":        "https://",
		"not a url":           "::not a url::",
	}

	for name, relayURL := range cases {
		t.Run(name, func(t *testing.T) {
			emailClient = nil
			t.Cleanup(func() { emailClient = nil })

			InitEmailClient(&appconfig.EmailConfig{RelayURL: relayURL, RelaySecret: testRelaySecret})
			assert.Nil(t, emailClient, "an unsafe relay URL must leave the client unset")

			require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
			WaitEmails()
		})
	}
}

func TestRelayBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://relay.example.com":       "https://relay.example.com",
		"https://relay.example.com/":      "https://relay.example.com",
		"  https://relay.example.com  ":   "https://relay.example.com",
		"https://relay.example.com:8443/": "https://relay.example.com:8443",
		"http://localhost:8080":           "http://localhost:8080",
		"http://127.0.0.1:8080/":          "http://127.0.0.1:8080",
	}

	for raw, want := range cases {
		got, err := relayBaseURL(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
}

// TestInitEmailClient_TrailingSlashBaseURL guards against a MAIL_RELAY_URL
// stored with a trailing slash producing a doubled procedure path.
func TestInitEmailClient_TrailingSlashBaseURL(t *testing.T) {
	handler := &recordingEmailHandler{}
	srv := newRelayStub(t, handler)
	initTestEmailClient(t, srv.URL+"/")

	require.NoError(t, SendEmailExt([]string{"user@example.com"}, "Your OTP", "<p>Code</p>", "", ""))
	WaitEmails()

	assert.Len(t, handler.received(), 1, "the trailing slash must not break the procedure path")
}
