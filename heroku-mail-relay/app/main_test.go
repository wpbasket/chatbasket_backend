package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	rpc_core_emailv1 "heroku-mail-relay/gen/proto/core/core_email"
	rpc_core_emailv1connect "heroku-mail-relay/gen/proto/core/core_email/rpc_core_emailv1connect"

	"connectrpc.com/connect"
)

const testSecret = "test-relay-secret"

// signingTransport signs outbound requests the way the backend's relaySigner
// does. The hooks exist so the negative paths can be exercised with a client
// that is otherwise correct.
type signingTransport struct {
	base   http.RoundTripper
	secret string
	nonce  string        // when set, reused on every request (replay tests)
	skew   time.Duration // applied to the signed timestamp
	mutate func(*http.Request)
}

func (t *signingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		read, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = read
	}

	nonce := t.nonce
	if nonce == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, err
		}
		nonce = hex.EncodeToString(raw[:])
	}
	timestamp := strconv.FormatInt(time.Now().Add(t.skew).Unix(), 10)

	signed := req.Clone(req.Context())
	signed.Body = io.NopCloser(bytes.NewReader(body))
	signed.ContentLength = int64(len(body))
	signed.Header.Set(relayTimestampHeader, timestamp)
	signed.Header.Set(relayNonceHeader, nonce)
	signed.Header.Set(relaySignatureHeader, relaySignatureVersion+"="+
		signRelayRequest([]byte(t.secret), timestamp, nonce, signed.URL.Path, body))

	if t.mutate != nil {
		t.mutate(signed)
	}
	return t.base.RoundTrip(signed)
}

// newTestRelay mounts the same routes as main() and returns the server plus a
// correctly signing client.
func newTestRelay(t *testing.T) (*httptest.Server, rpc_core_emailv1connect.EmailServiceClient) {
	t.Helper()

	srv := newTestRelayWithConfig(t, testConfig())
	return srv, newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: testSecret})
}

// newTestRelayWithConfig starts the relay with an explicit configuration.
func newTestRelayWithConfig(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()

	drainQueue()
	t.Cleanup(drainQueue)

	mux := http.NewServeMux()
	emailPath, emailHandler := rpc_core_emailv1connect.NewEmailServiceHandler(
		&emailServer{},
		connect.WithReadMaxBytes(maxRequestBytes),
	)
	mux.Handle(emailPath, relayGuard(cfg, emailHandler))
	mux.HandleFunc("/health", healthCheckHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newRelayClient(srv *httptest.Server, transport http.RoundTripper, opts ...connect.ClientOption) rpc_core_emailv1connect.EmailServiceClient {
	return rpc_core_emailv1connect.NewEmailServiceClient(&http.Client{Transport: transport}, srv.URL, opts...)
}

// drainQueue empties the package level job queue so tests do not leak state
// into one another.
func drainQueue() {
	for {
		select {
		case <-jobQueue:
		default:
			return
		}
	}
}

func validMessage() *rpc_core_emailv1.SendEmailRequest {
	return &rpc_core_emailv1.SendEmailRequest{
		To:       []string{"user@example.com"},
		Subject:  "Your OTP",
		Body:     "<p>Code: 123456</p>",
		TextBody: "Code: 123456",
		RefId:    "otp-login",
	}
}

func TestSendEmailQueuesJob(t *testing.T) {
	_, client := newTestRelay(t)

	res, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if err != nil {
		t.Fatalf("SendEmail returned error: %v", err)
	}

	if !res.Msg.GetQueued() {
		t.Error("expected queued to be true")
	}
	if got := res.Msg.GetQueueDepth(); got != 1 {
		t.Errorf("queue_depth = %d, want 1", got)
	}
	if got := res.Msg.GetQueueCapacity(); got != 200 {
		t.Errorf("queue_capacity = %d, want 200", got)
	}

	select {
	case job := <-jobQueue:
		if len(job.To) != 1 || job.To[0] != "user@example.com" {
			t.Errorf("job.To = %v, want [user@example.com]", job.To)
		}
		if job.Subject != "Your OTP" {
			t.Errorf("job.Subject = %q", job.Subject)
		}
		if job.Body != "<p>Code: 123456</p>" {
			t.Errorf("job.Body = %q", job.Body)
		}
		if job.TextBody != "Code: 123456" {
			t.Errorf("job.TextBody = %q", job.TextBody)
		}
		if job.RefID != "otp-login" {
			t.Errorf("job.RefID = %q", job.RefID)
		}
	default:
		t.Fatal("expected a job on the queue")
	}
}

// TestSendEmailRejectsUnsignedRequest proves the gateway is closed by default:
// a perfectly well formed call without a signature gets nowhere.
func TestSendEmailRejectsUnsignedRequest(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	client := newRelayClient(srv, srv.Client().Transport)

	_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeUnauthenticated, err)
	}
	if len(jobQueue) != 0 {
		t.Errorf("queue depth = %d, want 0", len(jobQueue))
	}
}

func TestSendEmailRejectsBadSignature(t *testing.T) {
	dropHeader := func(name string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Del(name) }
	}

	cases := map[string]*signingTransport{
		"wrong secret":        {secret: "not-the-secret"},
		"timestamp in past":   {secret: testSecret, skew: -5 * time.Minute},
		"timestamp in future": {secret: testSecret, skew: 5 * time.Minute},
		"no signature":        {secret: testSecret, mutate: dropHeader(relaySignatureHeader)},
		"no timestamp":        {secret: testSecret, mutate: dropHeader(relayTimestampHeader)},
		"no nonce":            {secret: testSecret, mutate: dropHeader(relayNonceHeader)},
		"short nonce":         {secret: testSecret, nonce: "tooshort"},
		"unknown version": {secret: testSecret, mutate: func(r *http.Request) {
			r.Header.Set(relaySignatureHeader, "v2=deadbeef")
		}},
		"tampered body": {secret: testSecret, mutate: func(r *http.Request) {
			tampered := []byte("tampered payload")
			r.Body = io.NopCloser(bytes.NewReader(tampered))
			r.ContentLength = int64(len(tampered))
		}},
	}

	for name, transport := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newTestRelayWithConfig(t, testConfig())
			transport.base = srv.Client().Transport
			client := newRelayClient(srv, transport)

			_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
			if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
				t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeUnauthenticated, err)
			}
			if len(jobQueue) != 0 {
				t.Errorf("queue depth = %d, want 0", len(jobQueue))
			}
		})
	}
}

// TestSendEmailRejectsReplay locks in the nonce cache: a signature is good for
// exactly one delivery, so a captured request cannot be sent twice.
func TestSendEmailRejectsReplay(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	client := newRelayClient(srv, &signingTransport{
		base:   srv.Client().Transport,
		secret: testSecret,
		nonce:  strings.Repeat("a", 32),
	})

	if _, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage())); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeUnauthenticated, err)
	}
	if len(jobQueue) != 1 {
		t.Errorf("queue depth = %d, want 1 (only the first call may be queued)", len(jobQueue))
	}
}

// TestSendEmailThrottlesRepeatedFailures shows the signing key cannot be
// probed at speed: after the budget is spent the caller is cut off outright.
func TestSendEmailThrottlesRepeatedFailures(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	client := newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: "wrong"})

	for i := 0; i < maxAuthFailures; i++ {
		_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
		if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
			t.Fatalf("attempt %d: code = %v, want %v", i+1, got, connect.CodeUnauthenticated)
		}
	}

	_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeResourceExhausted, err)
	}

	// A correctly signed call from the same address is blocked too, which is
	// the point: the throttle is on the address, not on the credential.
	good := newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: testSecret})
	if _, err := good.SendEmail(context.Background(), connect.NewRequest(validMessage())); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("code = %v, want %v", connect.CodeOf(err), connect.CodeResourceExhausted)
	}
}

// TestStaleSignedRequestsDoNotThrottle proves clock drift cannot lock the
// backend out of its own gateway: a stale request whose signature is valid
// comes from the key holder, so it is refused request by request without
// spending the failure budget, and delivery resumes the moment the clock is
// right again.
func TestStaleSignedRequestsDoNotThrottle(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	drifted := newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: testSecret, skew: -5 * time.Minute})

	for i := 0; i < maxAuthFailures+1; i++ {
		_, err := drifted.SendEmail(context.Background(), connect.NewRequest(validMessage()))
		if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
			t.Fatalf("attempt %d: code = %v, want %v (err: %v)", i+1, got, connect.CodeUnauthenticated, err)
		}
	}

	good := newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: testSecret})
	if _, err := good.SendEmail(context.Background(), connect.NewRequest(validMessage())); err != nil {
		t.Errorf("correctly signed call after the drift failed: %v", err)
	}
}

// TestStaleForgedRequestsStillThrottle shows a stale timestamp is no escape
// hatch: without a valid signature the request fails as a mismatch and the
// failure budget is still spent.
func TestStaleForgedRequestsStillThrottle(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	client := newRelayClient(srv, &signingTransport{base: srv.Client().Transport, secret: "wrong", skew: -5 * time.Minute})

	for i := 0; i < maxAuthFailures; i++ {
		_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
		if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
			t.Fatalf("attempt %d: code = %v, want %v (err: %v)", i+1, got, connect.CodeUnauthenticated, err)
		}
	}

	_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeResourceExhausted, err)
	}
}

func TestSendEmailSourceAddressAllowlist(t *testing.T) {
	allowed, err := parseAllowedIPs("203.0.113.5, 198.51.100.0/24")
	if err != nil {
		t.Fatalf("parseAllowedIPs failed: %v", err)
	}

	cfg := testConfig()
	cfg.AllowedIPs = allowed

	cases := map[string]struct {
		forwardedFor string
		wantCode     connect.Code
	}{
		"exact host allowed":     {forwardedFor: "203.0.113.5", wantCode: 0},
		"cidr member allowed":    {forwardedFor: "198.51.100.77", wantCode: 0},
		"foreign address denied": {forwardedFor: "203.0.113.9", wantCode: connect.CodePermissionDenied},
		// The router appends the real peer last, so a caller cannot talk its
		// way in by prefixing an allowed address of its own.
		"spoofed prefix denied":      {forwardedFor: "203.0.113.5, 45.66.77.88", wantCode: connect.CodePermissionDenied},
		"no forwarded header denied": {forwardedFor: "", wantCode: connect.CodePermissionDenied},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newTestRelayWithConfig(t, cfg)
			transport := &signingTransport{base: srv.Client().Transport, secret: testSecret}
			if tc.forwardedFor != "" {
				transport.mutate = func(r *http.Request) {
					r.Header.Set("X-Forwarded-For", tc.forwardedFor)
				}
			}

			_, err := newRelayClient(srv, transport).SendEmail(context.Background(), connect.NewRequest(validMessage()))
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("SendEmail returned error: %v", err)
				}
				return
			}
			if got := connect.CodeOf(err); got != tc.wantCode {
				t.Errorf("code = %v, want %v (err: %v)", got, tc.wantCode, err)
			}
			if len(jobQueue) != 0 {
				t.Errorf("queue depth = %d, want 0", len(jobQueue))
			}
		})
	}
}

// TestSendEmailForwardedProto refuses a request the router says arrived over
// plain HTTP. Only the last hop counts, so a caller cannot prepend "https".
func TestSendEmailForwardedProto(t *testing.T) {
	cases := map[string]struct {
		forwardedProto string
		wantCode       connect.Code
	}{
		"plaintext hop":  {forwardedProto: "http", wantCode: connect.CodePermissionDenied},
		"spoofed prefix": {forwardedProto: "https, http", wantCode: connect.CodePermissionDenied},
		"https hop":      {forwardedProto: "https", wantCode: 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newTestRelayWithConfig(t, testConfig())
			client := newRelayClient(srv, &signingTransport{
				base:   srv.Client().Transport,
				secret: testSecret,
				mutate: func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", tc.forwardedProto) },
			})

			_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("SendEmail returned error: %v", err)
				}
				return
			}
			if got := connect.CodeOf(err); got != tc.wantCode {
				t.Errorf("code = %v, want %v (err: %v)", got, tc.wantCode, err)
			}
		})
	}
}

// TestProcedureRejectsNonProtoRequests locks the endpoint down to binary
// protobuf and POST, and proves those checks run before authentication.
func TestProcedureRejectsNonProtoRequests(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())
	url := srv.URL + rpc_core_emailv1connect.EmailServiceSendEmailProcedure

	t.Run("json codec", func(t *testing.T) {
		res, err := srv.Client().Post(url, "application/json", strings.NewReader(`{"to":["user@example.com"]}`))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d, want %d", res.StatusCode, http.StatusUnsupportedMediaType)
		}
	})

	t.Run("get", func(t *testing.T) {
		res, err := srv.Client().Get(url)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", res.StatusCode, http.StatusMethodNotAllowed)
		}
	})
}

func TestSendEmailValidation(t *testing.T) {
	_, client := newTestRelay(t)

	tooManyRecipients := make([]string, 51)
	for i := range tooManyRecipients {
		tooManyRecipients[i] = "user@example.com"
	}

	cases := map[string]*rpc_core_emailv1.SendEmailRequest{
		"empty recipients":   {To: nil, Subject: "s", Body: "<p>b</p>"},
		"invalid address":    {To: []string{"not-an-email"}, Subject: "s", Body: "<p>b</p>"},
		"subject too long":   {To: []string{"user@example.com"}, Subject: strings.Repeat("x", 999), Body: "<p>b</p>"},
		"body too long":      {To: []string{"user@example.com"}, Subject: "s", Body: strings.Repeat("x", 500001)},
		"text body too long": {To: []string{"user@example.com"}, Subject: "s", Body: "<p>b</p>", TextBody: strings.Repeat("x", 500001)},
		"too many to-addrs":  {To: tooManyRecipients, Subject: "s", Body: "<p>b</p>"},
	}

	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.SendEmail(context.Background(), connect.NewRequest(msg))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeInvalidArgument, err)
			}
			if len(jobQueue) != 0 {
				t.Errorf("queue depth = %d, want 0", len(jobQueue))
			}
		})
	}
}

func TestSendEmailQueueFull(t *testing.T) {
	_, client := newTestRelay(t)

	for i := 0; i < cap(jobQueue); i++ {
		jobQueue <- emailJob{To: []string{"user@example.com"}}
	}

	_, err := client.SendEmail(context.Background(), connect.NewRequest(validMessage()))
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Fatalf("code = %v, want %v (err: %v)", got, connect.CodeResourceExhausted, err)
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("error is not a *connect.Error: %v", err)
	}
	if got := connectErr.Meta().Get("Retry-After"); got != "5" {
		t.Errorf("Retry-After = %q, want %q", got, "5")
	}
}

func TestSendEmailLargeBodyRoundTrip(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())

	// 500KB is the largest body the gateway accepts; gzip keeps it small on
	// the wire, which is exactly how the backend sends it.
	client := newRelayClient(srv,
		&signingTransport{base: srv.Client().Transport, secret: testSecret},
		connect.WithSendGzip())

	body := strings.Repeat("a", 500000)
	msg := validMessage()
	msg.Body = body

	if _, err := client.SendEmail(context.Background(), connect.NewRequest(msg)); err != nil {
		t.Fatalf("SendEmail returned error: %v", err)
	}

	job := <-jobQueue
	if len(job.Body) != len(body) {
		t.Errorf("body length = %d, want %d", len(job.Body), len(body))
	}
}

func TestSendEmailRejectsOversizedRequest(t *testing.T) {
	_, client := newTestRelay(t)

	// Uncompressed, this message is well beyond the wire cap, so the guard
	// must reject it instead of buffering it.
	msg := validMessage()
	msg.Body = strings.Repeat("a", 2<<20)

	_, err := client.SendEmail(context.Background(), connect.NewRequest(msg))
	if err == nil {
		t.Fatal("expected an error for an oversized request")
	}
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Errorf("code = %v, want %v (err: %v)", got, connect.CodeResourceExhausted, err)
	}
	if len(jobQueue) != 0 {
		t.Errorf("queue depth = %d, want 0", len(jobQueue))
	}
}

func TestHealthCheck(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())

	res, err := srv.Client().Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got, want := string(body), `{"status":"ok","queue":0,"capacity":200}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestHealthCheckDegraded(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())

	for i := 0; i < 181; i++ {
		jobQueue <- emailJob{To: []string{"user@example.com"}}
	}

	res, err := srv.Client().Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusServiceUnavailable)
	}
	if got, want := string(body), `{"status":"degraded","queue":181,"capacity":200}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// TestLegacyJSONRouteRemoved locks in the hard cutover: the old
// `POST /` JSON endpoint no longer exists.
func TestLegacyJSONRouteRemoved(t *testing.T) {
	srv := newTestRelayWithConfig(t, testConfig())

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/", strings.NewReader(`{"to":["user@example.com"],"subject":"s","body":"<p>b</p>"}`))
	if err != nil {
		t.Fatalf("building request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("legacy request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusNotFound)
	}
	if len(jobQueue) != 0 {
		t.Errorf("queue depth = %d, want 0", len(jobQueue))
	}
}

func TestParseAllowedIPs(t *testing.T) {
	networks, err := parseAllowedIPs(" 203.0.113.5 , 198.51.100.0/24 ,, 2001:db8::1 ")
	if err != nil {
		t.Fatalf("parseAllowedIPs failed: %v", err)
	}
	if len(networks) != 3 {
		t.Fatalf("parsed %d networks, want 3", len(networks))
	}

	for _, addr := range []string{"203.0.113.5", "198.51.100.1", "2001:db8::1"} {
		if !addrAllowed(networks, addr) {
			t.Errorf("%s should be allowed", addr)
		}
	}
	for _, addr := range []string{"203.0.113.6", "198.51.101.1", "2001:db8::2", "not-an-ip"} {
		if addrAllowed(networks, addr) {
			t.Errorf("%s should be denied", addr)
		}
	}

	if _, err := parseAllowedIPs("nonsense"); err == nil {
		t.Error("expected an error for a malformed entry")
	}
	if networks, err := parseAllowedIPs(""); err != nil || networks != nil {
		t.Errorf("an empty value must disable the allowlist, got %v (%v)", networks, err)
	}
}

func testConfig() Config {
	return Config{
		Secret:       testSecret,
		SMTPHost:     "smtp.zoho.in",
		SMTPPort:     "587",
		SMTPUser:     "noreply@example.com",
		SMTPPass:     "secret",
		SMTPFrom:     "noreply@example.com",
		SMTPFromName: "ChatBasket",
	}
}

// TestBuildMIMEMessageDerivesPlainText proves the MIME path is unaffected by
// the transport change: an empty text_body still yields a text/plain part.
func TestBuildMIMEMessageDerivesPlainText(t *testing.T) {
	msg, err := buildMIMEMessage(testConfig(), "user@example.com", "Your OTP", "<p>Code: <b>123456</b></p>", "", "otp-login")
	if err != nil {
		t.Fatalf("buildMIMEMessage failed: %v", err)
	}

	out := string(msg)
	for _, want := range []string{
		`Content-Type: text/plain; charset="UTF-8"`,
		`Content-Type: text/html; charset="UTF-8"`,
		"X-Entity-Ref-ID: otp-login",
		"Code: 123456",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("MIME message missing %q", want)
		}
	}
}

// TestBuildMIMEMessageStripsHeaderInjection guards the CR/LF sanitisation that
// used to sit behind the JSON handler.
func TestBuildMIMEMessageStripsHeaderInjection(t *testing.T) {
	msg, err := buildMIMEMessage(testConfig(), "user@example.com\r\nBcc: attacker@example.com", "OTP\r\nX-Evil: 1", "<p>hi</p>", "hi", "")
	if err != nil {
		t.Fatalf("buildMIMEMessage failed: %v", err)
	}

	// The injected text survives as inert characters on the original header
	// line; what must never happen is a new header line being created.
	out := string(msg)
	if strings.Contains(out, "\r\nBcc:") {
		t.Error("recipient CR/LF injection created a new header line")
	}
	if strings.Contains(out, "\r\nX-Evil:") {
		t.Error("subject CR/LF injection created a new header line")
	}
	if !strings.Contains(out, "To: user@example.comBcc: attacker@example.com\r\n") {
		t.Error("expected the recipient CR/LF to be stripped in place")
	}
}
