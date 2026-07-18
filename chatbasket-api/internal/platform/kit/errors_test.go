package kit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	rpc_common_errorv1 "chatbasket-api/gen/proto/common/error"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v5"
	"google.golang.org/protobuf/proto"
)

func TestActualServerErrors(t *testing.T) {
	if os.Getenv("RUN_UPGRADE_TESTS") != "true" {
		t.Skip("skipping actual server integration tests; set RUN_UPGRADE_TESTS=true to run")
	}

	// 1. Create a live Echo server
	e := echo.New()
	e.HTTPErrorHandler = GlobalErrorHandler

	// --- REST Endpoints (Testing GlobalErrorHandler, NewError, NewErrorWithDetails, GetPostgresError, GetStatusCodeFromError) ---

	// 1. Test NewError
	e.POST("/test-rest-new", func(c *echo.Context) error {
		return NewError(http.StatusConflict, "conflict_error", "resource conflict")
	})

	// 2. Test NewErrorWithDetails
	e.POST("/test-rest-details", func(c *echo.Context) error {
		details := map[string]string{"field": "username", "issue": "taken"}
		return NewErrorWithDetails(http.StatusBadRequest, "validation_error", "invalid request", details)
	})

	// 3. Test GetPostgresError mapping
	e.POST("/test-rest-pg", func(c *echo.Context) error {
		pgErr := &pgconn.PgError{
			Message:  "duplicate key value violates unique constraint",
			Severity: "ERROR",
		}
		err := GetPostgresError(pgErr)
		return NewError(http.StatusConflict, "database_error", err.Message)
	})

	// 4. Test GetStatusCodeFromError
	e.POST("/test-rest-status-code", func(c *echo.Context) error {
		return echo.NewHTTPError(http.StatusTeapot, "i am a teapot")
	})

	// 5. Test REST raw unexpected error fallback (GlobalErrorHandler Case 3)
	e.POST("/test-rest-raw", func(c *echo.Context) error {
		return errors.New("raw backend connection failure")
	})

	// 6. Test GetPostgresError fallback with a standard error
	e.POST("/test-rest-pg-fallback", func(c *echo.Context) error {
		err := GetPostgresError(errors.New("standard pg error message"))
		return NewError(http.StatusConflict, "database_error", err.Message)
	})

	// --- Connect RPC Mock endpoints mounted on Echo using the actual Connect library ---
	connectHandler := connect.NewUnaryHandler(
		"test.TestService/TestCall",
		func(ctx context.Context, req *connect.Request[rpc_common_errorv1.ErrorDetails]) (*connect.Response[rpc_common_errorv1.ErrorDetails], error) {
			scenario := req.Header().Get("X-Test-Scenario")
			var err error
			switch scenario {
			case "connect-new":
				// 5. Test NewConnectRpcError
				err = NewConnectRpcError(http.StatusForbidden, "forbidden_kind", "no permission")
			case "connect-details":
				// 6. Test NewConnectRpcErrorWithDetails
				mockDetail := &rpc_common_errorv1.ErrorDetails{
					Type: "nested_error_detail_type",
				}
				err = NewConnectRpcErrorWithDetails(
					http.StatusBadRequest,
					"validation_error",
					"invalid parameters",
					mockDetail,
				)
			case "connect-parse":
				// 7. Test ParseIntoRpcError with standard ProcessedError
				err = ParseIntoRpcError(NewError(http.StatusConflict, "conflict_error", "resource conflict"))
			case "connect-parse-raw":
				// 8. Test ParseIntoRpcError with raw error
				err = ParseIntoRpcError(errors.New("something raw"))
			case "connect-status-map":
				// 9. Test all HTTP status mappings
				status := http.StatusBadRequest
				switch req.Header().Get("X-Test-Status") {
				case "400":
					status = 400
				case "401":
					status = 401
				case "403":
					status = 403
				case "404":
					status = 404
				case "409":
					status = 409
				case "429":
					status = 429
				case "408":
					status = 408
				case "412":
					status = 412
				case "422":
					status = 422
				case "501":
					status = 501
				case "503":
					status = 503
				case "504":
					status = 504
				case "500":
					status = 500
				}
				err = ParseIntoRpcError(NewError(status, "test_kind", "test message"))
			}
			return nil, err
		},
	)
	e.Any("/test-connect/test.TestService/TestCall", echo.WrapHandler(connectHandler))

	// 2. Start the actual Echo server in the background on an available dynamic port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on dynamic port: %v", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler: e,
	}

	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()

	client := &http.Client{}
	baseURL := "http://" + listener.Addr().String()

	// Helper to perform POST requests to our test endpoints
	sendRequest := func(path string) (int, string) {
		resp, err := client.Post(baseURL+path, "application/json", nil)
		if err != nil {
			t.Fatalf("failed to make request to %s: %v", path, err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}

	// === Verify Outputs of ALL Error Functions ===

	// 1. Verify NewError + GlobalErrorHandler
	{
		status, body := sendRequest("/test-rest-new")
		if status != http.StatusConflict {
			t.Errorf("Test Case 1: expected status 409, got %d", status)
		}
		var restErr ApiError
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if restErr.Code != 409 || restErr.Type != "conflict_error" || restErr.Message != "resource conflict" {
				t.Errorf("Test Case 1: unexpected REST error output: %+v", restErr)
			}
		}
	}

	// 2. Verify NewErrorWithDetails + GlobalErrorHandler
	{
		status, body := sendRequest("/test-rest-details")
		if status != http.StatusBadRequest {
			t.Errorf("Test Case 2: expected status 400, got %d", status)
		}
		var restErr struct {
			Code    int               `json:"code"`
			Type    string            `json:"type"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if restErr.Code != 400 || restErr.Type != "validation_error" || restErr.Details["field"] != "username" {
				t.Errorf("Test Case 2: unexpected REST details error: %+v", restErr)
			}
		}
	}

	// 3. Verify GetPostgresError + GlobalErrorHandler
	{
		status, body := sendRequest("/test-rest-pg")
		if status != http.StatusConflict {
			t.Errorf("Test Case 3: expected status 409, got %d", status)
		}
		var restErr ApiError
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if !strings.Contains(restErr.Message, "duplicate key value violates unique constraint") {
				t.Errorf("Test Case 3: expected msg to contain PgError message, got: %+v", restErr)
			}
		}
	}

	// 4. Verify GetStatusCodeFromError + GlobalErrorHandler
	{
		status, body := sendRequest("/test-rest-status-code")
		if status != http.StatusTeapot {
			t.Errorf("Test Case 4: expected status 418, got %d", status)
		}
		var restErr ApiError
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if restErr.Code != 418 || restErr.Message != "i am a teapot" {
				t.Errorf("Test Case 4: unexpected REST echo error mapping: %+v", restErr)
			}
		}
	}

	// 5. Verify REST raw unexpected error fallback (GlobalErrorHandler Case 3)
	{
		status, body := sendRequest("/test-rest-raw")
		if status != http.StatusInternalServerError {
			t.Errorf("Test Case 4b: expected status 500, got %d", status)
		}
		var restErr ApiError
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if restErr.Code != 500 || restErr.Type != "internal_error" || restErr.Message != "An unexpected error occurred" {
				t.Errorf("Test Case 4b: unexpected raw REST error mapping: %+v", restErr)
			}
		}
	}

	// 6. Verify GetPostgresError fallback with standard error
	{
		status, body := sendRequest("/test-rest-pg-fallback")
		if status != http.StatusConflict {
			t.Errorf("Test Case 4c: expected status 409, got %d", status)
		}
		var restErr ApiError
		if err := json.Unmarshal([]byte(body), &restErr); err == nil {
			if restErr.Code != 409 || restErr.Type != "database_error" || restErr.Message != "standard pg error message" {
				t.Errorf("Test Case 4c: unexpected pg fallback REST error mapping: %+v", restErr)
			}
		}
	}

	// Helper to perform POST requests to our Connect endpoint
	sendConnectRequest := func(scenario string, statusHeader string) (int, string) {
		req, err := http.NewRequest("POST", baseURL+"/test-connect/test.TestService/TestCall", bytes.NewBuffer([]byte("{}")))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Scenario", scenario)
		if statusHeader != "" {
			req.Header.Set("X-Test-Status", statusHeader)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}

	// 5. Verify NewConnectRpcError over wire
	{
		status, body := sendConnectRequest("connect-new", "")
		if status != http.StatusForbidden {
			t.Errorf("Test Case 5: expected status 403, got %d", status)
		}
		var errData struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"details"`
		}
		if err := json.Unmarshal([]byte(body), &errData); err == nil {
			if errData.Code != "permission_denied" || errData.Message != "no permission" {
				t.Errorf("Test Case 5: unexpected Connect error: %+v", errData)
			}
			if len(errData.Details) != 1 || errData.Details[0].Type != "rpc_common_error.v1.ErrorDetails" {
				t.Errorf("Test Case 5: unexpected details: %+v", errData.Details)
			}
			decoded, _ := decodeBase64(errData.Details[0].Value)
			var ed rpc_common_errorv1.ErrorDetails
			if err2 := proto.Unmarshal(decoded, &ed); err2 == nil {
				if ed.Type != "forbidden_kind" {
					t.Errorf("Test Case 5: expected inner type 'forbidden_kind', got %q", ed.Type)
				}
			} else {
				t.Errorf("Test Case 5: failed to unmarshal proto: %v. Body: %s", err2, body)
			}
		}
	}

	// 6. Verify NewConnectRpcErrorWithDetails over wire
	{
		status, body := sendConnectRequest("connect-details", "")
		if status != http.StatusBadRequest {
			t.Errorf("Test Case 6: expected status 400, got %d", status)
		}
		var errData struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"details"`
		}
		if err := json.Unmarshal([]byte(body), &errData); err == nil {
			if errData.Code != "invalid_argument" || errData.Message != "invalid parameters" {
				t.Errorf("Test Case 6: unexpected Connect error: %+v", errData)
			}
			if len(errData.Details) != 1 || errData.Details[0].Type != "rpc_common_error.v1.ErrorDetails" {
				t.Errorf("Test Case 6: unexpected details: %+v", errData.Details)
			}
			decoded, _ := decodeBase64(errData.Details[0].Value)
			var ed rpc_common_errorv1.ErrorDetails
			if err2 := proto.Unmarshal(decoded, &ed); err2 == nil {
				if ed.Type != "validation_error" {
					t.Errorf("Test Case 6: expected inner type 'validation_error', got %q", ed.Type)
				}
				if ed.Details == nil {
					t.Errorf("Test Case 6: expected nested details, got nil")
				} else if ed.Details.TypeUrl != "type.googleapis.com/rpc_common_error.v1.ErrorDetails" {
					t.Errorf("Test Case 6: unexpected nested details type url: %q", ed.Details.TypeUrl)
				}
			} else {
				t.Errorf("Test Case 6: failed to unmarshal proto: %v", err2)
			}
		}
	}

	// 7. Verify ParseIntoRpcError with ProcessedError over wire
	{
		status, body := sendConnectRequest("connect-parse", "")
		if status != http.StatusConflict {
			t.Errorf("Test Case 7: expected status 409, got %d", status)
		}
		var errData struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"details"`
		}
		if err := json.Unmarshal([]byte(body), &errData); err == nil {
			if errData.Code != "already_exists" || errData.Message != "resource conflict" {
				t.Errorf("Test Case 7: unexpected Connect error: %+v", errData)
			}
			if len(errData.Details) != 1 {
				t.Errorf("Test Case 7: expected 1 detail, got %d", len(errData.Details))
			}
			decoded, _ := decodeBase64(errData.Details[0].Value)
			var ed rpc_common_errorv1.ErrorDetails
			if err2 := proto.Unmarshal(decoded, &ed); err2 == nil {
				if ed.Type != "conflict_error" {
					t.Errorf("Test Case 7: expected inner type 'conflict_error', got %q", ed.Type)
				}
			} else {
				t.Errorf("Test Case 7: failed to unmarshal proto: %v", err2)
			}
		}
	}

	// 8. Verify ParseIntoRpcError fallback with raw error over wire
	{
		status, body := sendConnectRequest("connect-parse-raw", "")
		if status != http.StatusInternalServerError {
			t.Errorf("Test Case 8: expected status 500, got %d", status)
		}
		var errData struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(body), &errData); err == nil {
			if errData.Code != "internal" || errData.Message != "something raw" {
				t.Errorf("Test Case 8: unexpected Connect error: %+v", errData)
			}
		}
	}

	// 9. Verify all HTTP status to Connect code mappings
	{
		statusTests := []struct {
			httpStatus  int
			connectCode string
		}{
			{http.StatusBadRequest, "invalid_argument"},
			{http.StatusUnauthorized, "unauthenticated"},
			{http.StatusForbidden, "permission_denied"},
			{http.StatusNotFound, "not_found"},
			{http.StatusConflict, "already_exists"},
			{http.StatusTooManyRequests, "resource_exhausted"},
			{http.StatusRequestTimeout, "deadline_exceeded"},
			{http.StatusPreconditionFailed, "failed_precondition"},
			{http.StatusUnprocessableEntity, "invalid_argument"},
			{http.StatusNotImplemented, "unimplemented"},
			{http.StatusServiceUnavailable, "unavailable"},
			{http.StatusGatewayTimeout, "deadline_exceeded"},
			{http.StatusInternalServerError, "internal"},
		}

		for _, tc := range statusTests {
			_, body := sendConnectRequest("connect-status-map", fmt.Sprintf("%d", tc.httpStatus))
			var errData struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal([]byte(body), &errData); err == nil {
				if errData.Code != tc.connectCode {
					t.Errorf("Mapping status %d: expected Connect code %q, got %q. Body: %s", tc.httpStatus, tc.connectCode, errData.Code, body)
				}
			} else {
				t.Fatalf("Mapping status %d: failed to parse JSON: %v. Body: %s", tc.httpStatus, err, body)
			}
		}
	}
}

func decodeBase64(s string) ([]byte, error) {
	if i := len(s) % 4; i != 0 {
		s += strings.Repeat("=", 4-i)
	}
	return base64.StdEncoding.DecodeString(s)
}
