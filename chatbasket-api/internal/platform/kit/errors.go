package kit

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v5"
)

// --- Fundamental Error Interfaces ---

// ProcessedError defines a standard interface for errors that have been "processed" from raw technical
// failures into high-level business failures that know their transport mapping.
type ProcessedError interface {
	error
	Status() int  // HTTP Status Code
	Kind() string // e.g., "NOT_FOUND", "FORBIDDEN" (matches ApiError.Type)
}

// DetailedProcessedError extends ProcessedError with structured details.
// Used for errors that need to return additional data to the client (e.g., StaleKeysError).
type DetailedProcessedError interface {
	ProcessedError
	Details() interface{} // Structured data included in the error response
}

// --- Standard Error Models (DTOs) ---

// ApiError is the standard JSON error response, ported from chatbasket-api/model/error.go
type ApiError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Details interface{} `json:"details,omitempty"`
}

// Error implements the standard Go error interface.
func (e *ApiError) Error() string { return e.Message }

// Status and Kind make ApiError satisfy the ProcessedError interface for global compatibility.
func (e *ApiError) Status() int  { return e.Code }
func (e *ApiError) Kind() string { return e.Type }

// --- Module Implementation (Smart Processed Errors) ---

// processedError is a concrete implementation of ProcessedError used by all modules.
type processedError struct {
	code    int
	errType string
	message string
}

func (e *processedError) Error() string { return e.message }
func (e *processedError) Status() int   { return e.code }
func (e *processedError) Kind() string  { return e.errType }

// NewError creates a new "Smart Processed Error" that implements kit.ProcessedError.
func NewError(code int, errType, message string) error {
	return &processedError{
		code:    code,
		errType: errType,
		message: message,
	}
}

// --- Global Handlers (The "Brain") ---

// GlobalErrorHandler is the central error processing point for the entire application,
// registered with Echo via e.HTTPErrorHandler.
func GlobalErrorHandler(c *echo.Context, err error) {
	if err == nil {
		return
	}

	// 0. Ensure we don't write the response multiple times in the middleware chain.
	if resp, uErr := echo.UnwrapResponse(c.Response()); uErr == nil {
		if resp.Committed {
			return // response has been already sent to the client by handler or some middleware
		}
	}

	// Handle HEAD requests specifically to avoid body transmission
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(GetStatusCodeFromError(err))
		return
	}

	// 1. Check if it's a ProcessedError (or a compatible ApiError)
	var pe ProcessedError
	if errors.As(err, &pe) {
		apiErr := ApiError{
			Code:    pe.Status(),
			Type:    pe.Kind(),
			Message: pe.Error(),
		}
		// Check if error has structured details
		if dpe, ok := pe.(DetailedProcessedError); ok {
			apiErr.Details = dpe.Details()
		}
		_ = c.JSON(pe.Status(), apiErr)
		return
	}

	// 2. Check if it's an Echo HTTP Error (e.g., 404/405 from router, or binding/validation errors)
	var he *echo.HTTPError
	if errors.As(err, &he) {
		message := he.Message
		if message == "" {
			message = he.Error()
		}
		_ = c.JSON(he.Code, ApiError{
			Code:    he.Code,
			Type:    "http_error",
			Message: message,
		})
		return
	}

	// 3. Fallback: Internal Server Error (Something secret or unexpected failed)
	slog.Error("Unexpected API Error",
		"error", err,
		"path", c.Request().URL.Path,
		"method", c.Request().Method,
	)

	_ = c.JSON(http.StatusInternalServerError, ApiError{
		Code:    http.StatusInternalServerError,
		Type:    "internal_error",
		Message: "An unexpected error occurred",
	})
}

// --- External Adaptors (Library Helpers) ---

// GetStatusCodeFromError extracts the HTTP status code from an error, ported from utils/errorUtils.go
func GetStatusCodeFromError(err error) int {
	// Go 1.26: Using errors.AsType[T] for type-safe error handling
	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		return he.Code
	}
	return http.StatusInternalServerError // fallback for non-HTTP errors
}

// PostgresError is specialized for database failures, ported from utils/errorUtils.go
type PostgresError struct {
	Message string
	PgError *pgconn.PgError
}

// GetPostgresError extracts pgx-specific error details, ported from utils/errorUtils.go
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
