package kit

import (
	"errors"
	"log/slog"
	"net/http"

	rpc_common_errorv1 "chatbasket-api/gen/proto/common/error"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
// Used for errors that need to return additional data to the client (e.g., a
// chat keys-update error). The global error handler calls Details() to include
// the value in the JSON response's "details" field.
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
	details interface{}
}

func (e *processedError) Error() string        { return e.message }
func (e *processedError) Status() int          { return e.code }
func (e *processedError) Kind() string         { return e.errType }
func (e *processedError) Details() interface{} { return e.details }

// NewError creates a new "Smart Processed Error" that implements kit.ProcessedError.
func NewError(code int, errType, message string) error {
	return &processedError{
		code:    code,
		errType: errType,
		message: message,
	}
}

// NewErrorWithDetails creates a ProcessedError that also implements
// DetailedProcessedError. The details value is included in the JSON
// response by the global error handler. Use this whenever the frontend
// needs structured data about the error beyond the message.
func NewErrorWithDetails(code int, errType, message string, details interface{}) error {
	return &processedError{
		code:    code,
		errType: errType,
		message: message,
		details: details,
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

// NewConnectRpcError maps a standard kit.ProcessedError status code to its Connect RPC equivalent code,
// packages detailed metadata using commonpb.ErrorDetails, and serializes any inner details to JSON.
func NewConnectRpcError(err error) error {
	if err == nil {
		return nil
	}
	// Check if the error is already a connect.Error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}
	// Map kit.ProcessedError status codes to Connect codes
	var pe ProcessedError
	if errors.As(err, &pe) {
		var code connect.Code
		switch pe.Status() {
		case http.StatusBadRequest:
			code = connect.CodeInvalidArgument
		case http.StatusUnauthorized:
			code = connect.CodeUnauthenticated
		case http.StatusForbidden:
			code = connect.CodePermissionDenied
		case http.StatusNotFound:
			code = connect.CodeNotFound
		case http.StatusConflict:
			code = connect.CodeAlreadyExists
		case http.StatusTooManyRequests:
			code = connect.CodeResourceExhausted
		case http.StatusRequestTimeout:
			code = connect.CodeDeadlineExceeded
		case http.StatusPreconditionFailed:
			code = connect.CodeFailedPrecondition
		case http.StatusUnprocessableEntity:
			code = connect.CodeInvalidArgument
		case http.StatusNotImplemented:
			code = connect.CodeUnimplemented
		case http.StatusServiceUnavailable:
			code = connect.CodeUnavailable
		case http.StatusGatewayTimeout:
			code = connect.CodeDeadlineExceeded
		default:
			code = connect.CodeInternal
		}

		newErr := connect.NewError(code, errors.New(pe.Error()))

		// Build ErrorDetails protobuf payload (type + details only; code and message live in the Connect envelope)
		detailsPayload := &rpc_common_errorv1.ErrorDetails{
			Type: pe.Kind(),
		}

		// Extract and serialize structured details if available
		if dpe, ok := pe.(DetailedProcessedError); ok && dpe.Details() != nil {
			if protoMsg, ok := dpe.Details().(proto.Message); ok {
				if detailsAny, anyErr := anypb.New(protoMsg); anyErr == nil {
					detailsPayload.Details = detailsAny
				}
			}
		}

		if detailAny, detailErr := connect.NewErrorDetail(detailsPayload); detailErr == nil {
			newErr.AddDetail(detailAny)
		}

		return newErr
	}
	return connect.NewError(connect.CodeInternal, err)
}
