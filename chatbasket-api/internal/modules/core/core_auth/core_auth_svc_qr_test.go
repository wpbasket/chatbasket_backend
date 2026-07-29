package core_auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAuthServiceQR(t *testing.T) (*AuthService, pgxmock.PgxPoolIface) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	store := core_auth_store.New(mock)
	svc := &AuthService{
		GlobalService:   services.NewGlobalService("https://chatbasket.live"),
		PostgresQuerier: store,
		PostgresQueries: store,
		Pool:            nil,
		AuthSecret:      []byte("test-secret"),
	}
	return svc, mock
}

func signToken(t *testing.T, id uuid.UUID, secret []byte) string {
	sig, err := kit.ComputeHMAC(id.String(), secret, false, nil)
	require.NoError(t, err)
	return fmt.Sprintf("%s.%s", id.String(), sig)
}

func TestQRInitiate_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	mock.ExpectQuery("INSERT INTO qr_login_requests").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(id))

	resp, err := svc.QRInitiate(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	
	parsedID, err := svc.ParseAndVerifyQRToken(resp.QrToken)
	assert.NoError(t, err)
	assert.Equal(t, id, parsedID)
	assert.Equal(t, int32(300), resp.ExpiresIn)
}

func TestQRInitiate_DBError(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	mock.ExpectQuery("INSERT INTO qr_login_requests").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("db insert error"))

	resp, err := svc.QRInitiate(context.Background())
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRApprove_InvalidUUID(t *testing.T) {
	svc, _ := setupTestAuthServiceQR(t)
	resp, err := svc.QRApprove(context.Background(), uuid.New(), "invalid")
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRApprove_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	userID := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("UPDATE qr_login_requests").
		WithArgs(id, &userID).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(id))

	mock.ExpectExec("SELECT pg_notify").
		WithArgs(fmt.Sprintf("%s_approve", id.String())).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))

	resp, err := svc.QRApprove(context.Background(), userID, signedToken)

	assert.NoError(t, err)
	assert.True(t, resp.Status)
}

func TestQRApprove_NotifyError(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	userID := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("UPDATE qr_login_requests").
		WithArgs(id, &userID).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(id))

	mock.ExpectExec("SELECT pg_notify").
		WithArgs(fmt.Sprintf("%s_approve", id.String())).
		WillReturnError(errors.New("notify failed"))

	resp, err := svc.QRApprove(context.Background(), userID, signedToken)

	assert.ErrorContains(t, err, "notify")
	assert.Nil(t, resp)
}

func TestQRApprove_DBError(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	userID := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("UPDATE qr_login_requests").
		WithArgs(id, &userID).
		WillReturnError(errors.New("db error"))

	resp, err := svc.QRApprove(context.Background(), userID, signedToken)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRCallback_InvalidUUID(t *testing.T) {
	svc, _ := setupTestAuthServiceQR(t)
	resp, err := svc.QRCallback(context.Background(), "invalid", "web")
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRCallback_ExpiredOrNotApproved(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("UPDATE qr_login_requests").
		WithArgs(id).
		WillReturnError(pgx.ErrNoRows)

	resp, err := svc.QRCallback(context.Background(), signedToken, "web")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRCallback_MissingUserID(t *testing.T) {
	svc, mock := setupTestAuthServiceQR(t)
	defer mock.Close()

	id := uuid.New()
	signedToken := signToken(t, id, svc.AuthSecret)

	mock.ExpectQuery("UPDATE qr_login_requests").
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"auth_user_id"}).AddRow(nil))

	resp, err := svc.QRCallback(context.Background(), signedToken, "web")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestQRApprove_InvalidSignature(t *testing.T) {
	svc, _ := setupTestAuthServiceQR(t)
	id := uuid.New()
	userID := uuid.New()
	invalidToken := fmt.Sprintf("%s.invalid-signature-value-here", id.String())

	resp, err := svc.QRApprove(context.Background(), userID, invalidToken)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "signature")
	assert.Nil(t, resp)
}

func TestQRCallback_InvalidSignature(t *testing.T) {
	svc, _ := setupTestAuthServiceQR(t)
	id := uuid.New()
	invalidToken := fmt.Sprintf("%s.invalid-signature-value-here", id.String())

	resp, err := svc.QRCallback(context.Background(), invalidToken, "web")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "signature")
	assert.Nil(t, resp)
}
