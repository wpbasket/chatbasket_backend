package utils

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

// GetStatusCodeFromError extracts the HTTP status code from an error.
func GetStatusCodeFromError(err error) int {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he.Code
	}
	return http.StatusInternalServerError // fallback for non-HTTP errors
}

type PostgresError struct {
	Message string
	PgError *pgconn.PgError
}

func GetPostgresError(err error) *PostgresError {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &PostgresError{Message: pgErr.Message, PgError: pgErr}
	}
	return &PostgresError{Message: err.Error(), PgError: nil}
}
