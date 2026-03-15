package utils

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

// GetStatusCodeFromError extracts the HTTP status code from an error.
func GetStatusCodeFromError(err error) int {
	// Go 1.26: Using errors.AsType[T] for type-safe error handling
	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
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

	// Go 1.26: Using errors.AsType[T] for type-safe error handling
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return &PostgresError{Message: pgErr.Message, PgError: pgErr}
	}
	return &PostgresError{Message: err.Error(), PgError: nil}
}
