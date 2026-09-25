package middleware

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Register adds the shared middlewares to the server.
// These run for every request: logging, recover, CORS, rate limit, and gzip.
func Register(e *echo.Echo, corsOrigin string) {
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// Allow the frontend to call the API from a browser.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{corsOrigin},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposeHeaders:    []string{"Connect-Content-Encoding", "Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: true,
		MaxAge:           86400, // Remember preflight answers for 24 hours.
	}))

	// Allow 100 requests per second per IP.
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
	e.Use(middleware.Secure())

	// Shrink normal answers with gzip.
	// Skip it for live connections (websocket, RPC, SSE) because they stream.
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c *echo.Context) bool {
			if c.Request().Header.Get("Upgrade") == "websocket" {
				return true
			}
			ct := c.Request().Header.Get("Content-Type")
			p := c.Request().URL.Path
			return strings.HasPrefix(ct, "application/grpc") ||
				strings.HasPrefix(ct, "application/connect") ||
				strings.HasPrefix(ct, "application/proto") ||
				strings.Contains(p, "personal_sse") ||
				strings.Contains(p, "StreamEvents")
		},
	}))

}

// ContextTimeout stops a request that takes too long.
// Example: ContextTimeout(30 seconds) ends slow requests with a 503 error.
func ContextTimeout(timeout time.Duration) echo.MiddlewareFunc {
	return middleware.ContextTimeout(timeout)
}

// ContextTimeoutWithSkipper is the same as ContextTimeout, but some paths skip it.
// The skipper returns true when the timeout should NOT run.
// Example: skip websocket paths, because they stay open for a long time.
func ContextTimeoutWithSkipper(timeout time.Duration, skipper middleware.Skipper) echo.MiddlewareFunc {
	return middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: timeout,
		Skipper: skipper,
	})
}

// BodyLimit blocks uploads that are too big.
// Example: BodyLimit(5MB) answers 413 when the body is bigger than 5MB.
func BodyLimit(limitBytes int64) echo.MiddlewareFunc {
	return middleware.BodyLimit(limitBytes)
}

// BodyLimitWithSkipper is the same as BodyLimit, but some paths skip it.
// Example: skip the history-sync upload path, because it needs 90MB.
func BodyLimitWithSkipper(limitBytes int64, skipper middleware.Skipper) echo.MiddlewareFunc {
	return middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		LimitBytes: limitBytes,
		Skipper:    skipper,
	})
}

// BodyLimitOverrides lists the few RPCs that may upload more than normal.
// Key = end of the RPC path. Value = allowed bytes.
// Example: {"/rpc_personal_chat.v1.ChatService/UploadHistorySync": 90MB}.
type BodyLimitOverrides = map[string]int64

// bodyLimitRule is one exception from the list above,
// with its limit already built and ready to use.
type bodyLimitRule struct {
	suffix string
	check  echo.MiddlewareFunc
}

// DynamicBodyLimit gives every RPC the normal upload cap,
// except the listed RPCs which get their own bigger cap.
//
// Why we need it: all RPCs of one service share a single Echo route
// (Any(path+"*")), so Echo cannot set a limit per RPC for us.
// This looks at the request path and picks the right limit.
//
// What happens:
//   - Normal RPC -> defaultLimit (for example 5MB).
//   - Listed RPC (for example UploadHistorySync) -> its own limit (90MB).
//   - Too big -> server answers 413 and the handler never runs.
//
// Speed: limits are built once at startup. Each request only compares
// the end of the path against 1-2 entries, longest match first.
func DynamicBodyLimit(defaultLimit int64, overrides BodyLimitOverrides) echo.MiddlewareFunc {
	defaultCheck := middleware.BodyLimit(defaultLimit)
	rules := make([]bodyLimitRule, 0, len(overrides))
	for suffix, limit := range overrides {
		rules = append(rules, bodyLimitRule{suffix: suffix, check: middleware.BodyLimit(limit)})
	}
	// Longest ending first, so the closest match always wins.
	sort.Slice(rules, func(i, j int) bool { return len(rules[i].suffix) > len(rules[j].suffix) })
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := c.Request().URL.Path
			for _, r := range rules {
				if strings.HasSuffix(path, r.suffix) {
					return r.check(next)(c)
				}
			}
			return defaultCheck(next)(c)
		}
	}
}

// ContextTimeoutOverrides lists the few RPCs that may run longer than normal.
// Key = end of the RPC path. Value = allowed time.
// Example: {"/rpc_personal_chat.v1.ChatService/UploadHistorySync": 10 minutes}.
type ContextTimeoutOverrides = map[string]time.Duration

// timeoutRule is one exception from the list above,
// with its timeout already built and ready to use.
type timeoutRule struct {
	suffix string
	check  echo.MiddlewareFunc
}

// DynamicContextTimeout gives every RPC the normal time limit,
// except the listed RPCs which get more time.
//
// Why we need it: all RPCs of one service share a single Echo route
// (Any(path+"*")), so Echo cannot set a timeout per RPC for us.
// This looks at the request path and picks the right timeout.
//
// What happens:
//   - Normal RPC -> defaultTimeout (for example 30 seconds).
//   - Listed RPC (for example UploadHistorySync) -> its own time (10 minutes).
//   - Too slow -> the request time ends and the server answers 503.
//     This only works if the handler watches the request context.
//
// Speed: timeouts are built once at startup. Each request only compares
// the end of the path against 1-2 entries, longest match first.
func DynamicContextTimeout(defaultTimeout time.Duration, overrides ContextTimeoutOverrides) echo.MiddlewareFunc {
	defaultCheck := middleware.ContextTimeout(defaultTimeout)
	rules := make([]timeoutRule, 0, len(overrides))
	for suffix, timeout := range overrides {
		rules = append(rules, timeoutRule{suffix: suffix, check: middleware.ContextTimeout(timeout)})
	}
	// Longest ending first, so the closest match always wins.
	sort.Slice(rules, func(i, j int) bool { return len(rules[i].suffix) > len(rules[j].suffix) })
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := c.Request().URL.Path
			for _, r := range rules {
				if strings.HasSuffix(path, r.suffix) {
					return r.check(next)(c)
				}
			}
			return defaultCheck(next)(c)
		}
	}
}
