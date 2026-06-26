package core_auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupQRWebSocketTest(t *testing.T, expiresAt time.Time) (*httptest.Server, *QRHub, uuid.UUID, string, pgxmock.PgxPoolIface) {
	svc, mock := setupTestAuthServiceQR(t)

	id := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("SELECT id, auth_user_id, status, expires_at FROM qr_login_requests").
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "auth_user_id", "status", "expires_at"}).
			AddRow(id, nil, "PENDING", expiresAt))

	hub := NewQRHub()
	handler := newAuthHandler(svc, nil, hub)

	e := echo.New()
	e.GET("/api/auth/qr/ws", handler.QRWebSocket)

	server := httptest.NewServer(e)
	t.Cleanup(server.Close)

	return server, hub, id, signedToken, mock
}

func buildQRWSURL(server *httptest.Server, token string) string {
	return strings.Replace(server.URL, "http", "ws", 1) + "/api/auth/qr/ws?token=" + token
}

func originHeader() http.Header {
	h := make(http.Header)
	h.Set("Origin", "https://chatbasket.live")
	return h
}

// TestQRWebSocket_ClosesOnExpiry verifies the WebSocket closes when the QR token expires.
func TestQRWebSocket_ClosesOnExpiry(t *testing.T) {
	server, _, _, signedToken, mock := setupQRWebSocketTest(t, time.Now().Add(200*time.Millisecond))
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, buildQRWSURL(server, signedToken), &websocket.DialOptions{
		HTTPHeader: originHeader(),
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	start := time.Now()
	_, _, err = conn.Read(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "expected connection to close after expiry")
	assert.Less(t, elapsed, 1*time.Second, "connection should close near expiry, not hang")
}

// TestQRWebSocket_ClosesOnSuccess verifies QRHub.Close terminates the waiting WebSocket.
func TestQRWebSocket_ClosesOnSuccess(t *testing.T) {
	server, hub, id, signedToken, mock := setupQRWebSocketTest(t, time.Now().Add(5*time.Minute))
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, buildQRWSURL(server, signedToken), &websocket.DialOptions{
		HTTPHeader: originHeader(),
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Simulate QRCallback completing and telling the hub to close the connection.
	go func() {
		time.Sleep(100 * time.Millisecond)
		hub.Close(id)
	}()

	start := time.Now()
	_, _, err = conn.Read(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "expected connection to close when hub.Close is called")
	assert.Less(t, elapsed, 500*time.Millisecond, "connection should close immediately on success")
}

// TestQRWebSocket_RegisterReplacesExisting verifies a second connection for the same token evicts the first.
func TestQRWebSocket_RegisterReplacesExisting(t *testing.T) {
	server, _, id, signedToken, mock := setupQRWebSocketTest(t, time.Now().Add(5*time.Minute))
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	first, _, err := websocket.Dial(ctx, buildQRWSURL(server, signedToken), &websocket.DialOptions{
		HTTPHeader: originHeader(),
	})
	require.NoError(t, err)
	defer first.Close(websocket.StatusNormalClosure, "")

	// Give the server time to register the first connection.
	time.Sleep(100 * time.Millisecond)

	// Second upgrade must also query the DB for the same token.
	mock.ExpectQuery("SELECT id, auth_user_id, status, expires_at FROM qr_login_requests").
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "auth_user_id", "status", "expires_at"}).
			AddRow(id, nil, "PENDING", time.Now().Add(5*time.Minute)))

	second, _, err := websocket.Dial(ctx, buildQRWSURL(server, signedToken), &websocket.DialOptions{
		HTTPHeader: originHeader(),
	})
	require.NoError(t, err)
	defer second.Close(websocket.StatusNormalClosure, "")

	// The first connection should have been kicked out by Register.
	start := time.Now()
	_, _, err = first.Read(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "expected first connection to be closed when second connection registered")
	assert.Less(t, elapsed, 500*time.Millisecond, "first connection should close immediately")
}
