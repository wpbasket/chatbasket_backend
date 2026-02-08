# Backend Consistency Guide

This document captures key patterns, conventions, and interdependencies for the ChatBasket backend. Use it to keep new code aligned with existing architecture.

## Architecture & Layering
- Flow: HTTP -> routes -> handler -> service -> db queries (sqlc).
- **Handler-Service Separation Pattern (CRITICAL)**:
  - **Handlers**: Only basic validation, context extraction, payload binding, service calls, response handling
  - **Services**: All business logic, UUID parsing, data validation, response mapping, heavy lifting
  - **Never put business logic in handlers** - keep them thin and consistent
- Handlers: Thin orchestration; translate HTTP input -> service calls; set cookies for web.
- Services: Business logic; use utils for hashing/OTP/session creation; return typed `model` structs.
- Repositories: sqlc-generated packages in `internal/db/...`; avoid ad-hoc SQL in handlers/services.

### Handler Responsibilities (Standard Pattern)
```go
func (h *Handler) Method(c echo.Context) error {
    // 1. Extract and validate context fields
    userId, ok := c.Get("userId").(string)
    if !ok || userId == "" {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{...})
    }
    uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
    if !okUUID {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{...})
    }

    // 2. Bind and validate request payload
    var payload personalmodel.Payload
    if err := c.Bind(&payload); err != nil {
        return c.JSON(http.StatusBadRequest, &model.ApiError{...})
    }

    // 3. Call service method
    resp, apiErr := h.service.Method(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
    if apiErr != nil {
        return c.JSON(apiErr.Code, apiErr)
    }

    // 4. Return response
    return c.JSON(http.StatusOK, resp)
}
```

### Service Responsibilities (Standard Pattern)
```go
func (ps *Service) Method(ctx context.Context, payload *personalmodel.Payload, userId model.UserId) (*personalmodel.Response, *model.ApiError) {
    // 1. Parse and validate UUIDs from payload
    id, err := uuid.Parse(payload.ID)
    if err != nil {
        return nil, &model.ApiError{...}
    }

    // 2. Perform business logic validation
    if validationFails {
        return nil, &model.ApiError{...}
    }

    // 3. Execute database operations
    data, err := ps.Queries.Operation(ctx, params)
    if err != nil {
        return nil, &model.ApiError{...}
    }

    // 4. Map to response models - IMPORTANT: Convert UUIDs to strings for frontend
    return &personalmodel.Response{
        ID:     data.ID.String(),  // Convert UUID to string
        UserID: data.UserID.String(), // Convert UUID to string
    }, nil
}
```

### UUID Handling Rule (CRITICAL)
- **Database**: Store UUIDs as native `uuid.UUID` types
- **Service Layer**: Parse UUIDs from strings, work with `uuid.UUID` internally
- **Frontend API**: Always return UUIDs as strings in JSON responses
- **Never return raw UUID objects** to frontend - always use `.String()` conversion

### Response Struct Rule (CRITICAL)
- **Always return custom structs** to frontend, never `map[string]interface{}`
- **Service Layer**: Return typed response structs (`*personalmodel.Response`)
- **Handler Layer**: Return the struct directly to JSON encoder
- **Benefits**: Type safety, API documentation, IDE autocomplete, consistent responses
- **Exception**: Only use `map[string]interface{}` for dynamic wrapper objects (like `{"messages": [...], "count": 5}`)

## Auth & Session Pattern
- OTP-first: Signup/Login require OTP verification (`/auth/signup-verification`, `/auth/login-verification`).
- Hashing: Argon2id for passwords and OTP hashes (`utils/auth_flow_utils.go`).
- Sessions: `CreateSessionFlow` issues token + 3y expiry, stores HMAC hash; auto-promotes first native session to primary (`is_central`).
- Web vs Native:
  - Web: session token stays in HttpOnly cookie; SessionResponse omits sensitive IDs for web clients.
  - Native: bearer header `Authorization: Bearer <sessionId>:<userId>` expected on API calls.
- Primary device metadata: `SessionResponse` exposes `isPrimary`, `primaryDeviceName` for client UI prompts.

## Middleware & Error Model
- Centralized error model (`model.ApiError`) for consistent JSON responses.
- Keep middleware for auth/rate-limit cross-cutting concerns; keep handlers minimal.

## Database & Queries
- sqlc packages: `internal/db/auth` (users, sessions, verification_codes), `internal/db/personal`.
- Sessions table: `token_hash`, `platform`, `device_token`, `device_name`, `is_central` for primary device tracking.
- Verification codes: keyed by user ID + type with expiry enforcement in utils.
- Add/modify SQL in `db/.../queries`, then regenerate sqlc; avoid inline SQL elsewhere.

## Utils
- `utils/auth_flow_utils.go`: OTP verify (3 min expiry), session create, HMAC helpers.
- Shared helpers live in `utils/`; keep services thin.

## Runtime Safety Requirements (CRITICAL)

### Type Assertion Safety
- **All type assertions MUST use the two-value form with `ok` check**
- **Never use single-value type assertions in production code**
- **Return consistent error responses for failed assertions**

```go
// ✅ SAFE PATTERN
userId, ok := c.Get("userId").(string)
if !ok || userId == "" {
    return c.JSON(http.StatusInternalServerError, &model.ApiError{
        Code:    http.StatusInternalServerError,
        Message: "Invalid user context",
        Type:    "internal_server_error",
    })
}

// ❌ UNSAFE - NEVER USE
userId := c.Get("userId").(string) // Can panic!
```

### Pointer Safety
- **Always check for nil before dereferencing pointers**
- **Use defensive programming for optional fields**
- **Provide meaningful fallback values where appropriate**

```go
// ✅ SAFE PATTERN
if tokenID != nil && *tokenID != "" {
    // Safe dereference after nil check
    result := *tokenID
}

// ❌ UNSAFE - NEVER USE
result := *tokenID // Can panic if tokenID is nil!
```

### Context Extraction Safety
- **Always validate context extraction with proper type checking**
- **Check multiple context values together when related**
- **Return consistent error responses for invalid context**

```go
// ✅ SAFE PATTERN
userId, ok := c.Get("userId").(string)
uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
if !ok || !okUUID {
    return c.JSON(http.StatusInternalServerError, &model.ApiError{
        Code:    http.StatusInternalServerError,
        Message: "Invalid user context",
        Type:    "internal_server_error",
    })
}
```

### Goroutine Safety
- **Always implement panic recovery in goroutines**
- **Use WaitGroup for proper synchronization**
- **Log panics for debugging purposes**

```go
// ✅ SAFE PATTERN
go func() {
    defer emailWG.Done()
    defer func() {
        if r := recover(); r != nil {
            fmt.Printf("Recovered from panic: %v\n", r)
        }
    }()
    // Goroutine work here
}()
```

### Database Transaction Safety
- **Always handle database errors with proper error wrapping**
- **Use consistent error message formatting**
- **Return appropriate HTTP status codes**

```go
// ✅ SAFE PATTERN
result, err := ps.PersonalQueries.CreateAvatar(ctx, params)
if err != nil {
    return nil, &model.ApiError{
        Code:    http.StatusInternalServerError,
        Message: "Failed to create avatar: " + utils.GetPostgresError(err).Message,
        Type:    "internal_server_error",
    }
}
```

## Security Notes
- OTPs hashed with Argon2id; expired codes rejected.
- Session tokens stored as HMAC hashes; validate via DB lookups.
- Web cookies are Secure/HttpOnly; native uses headers.
- Middleware validates sessions for both web (cookies) and native (bearer header), re-HMACs the token, and enforces expiry via `CheckSessionIsValid`.
- Gaps to consider: add rate limiting / OTP resend throttling and (if needed) email-verified checks in middleware.

## Infrastructure & Deployment
- Echo v4 HTTP server; see `Dockerfile` for production build.
- Env via `.env`; never hardcode secrets.
- Email gateway (Heroku worker) handles SMTP relay; API remains provider-agnostic.

## Contribution Checklist
- **CRITICAL: Follow Handler-Service Separation Pattern**:
  - Handlers: Only context validation, payload binding, service calls, response handling
  - Services: All business logic, UUID parsing, data validation, response mapping
  - Never put business logic in handlers
- **CRITICAL: UUID Handling Rule**:
  - Database: Store as native `uuid.UUID`
  - Service Layer: Parse from strings, work with `uuid.UUID` internally  
  - Frontend API: Always return UUIDs as strings using `.String()` conversion
  - Never return raw UUID objects to frontend
- **CRITICAL: Response Struct Rule**:
  - Always return custom structs to frontend, never `map[string]interface{}`
  - Service Layer: Return typed response structs
  - Exception: Only use `map[string]interface{}` for dynamic wrapper objects (like `{"messages": [...], "count": 5}`)
- **CRITICAL: Runtime Safety Requirements**:
  - Type assertions MUST use two-value form with `ok` check
  - Always check for nil before dereferencing pointers
  - Validate context extraction with proper type checking
  - Implement panic recovery in goroutines
  - Handle database errors with proper wrapping
- Business logic in services; handlers stay slim/typed.
- SQL changes via `db/.../queries` + sqlc regen.
- Keep responses typed in `model/`; avoid `map[string]interface{}` except for wrapper objects.
- When changing auth/session flows, update handler/service/utils and web vs native cookie/header behavior; preserve primary-device rules.
- Update docs when flows change.
- **Reference Examples**: See `contact_handler.go` and `chat_handler.go` for correct pattern implementation.
