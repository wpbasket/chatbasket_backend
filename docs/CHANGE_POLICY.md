# Change Policy (Backend)

Before changing backend code (API, services, middleware, db/sqlc, routes, handlers), follow this checklist to keep layers and docs consistent.

## Pre-change review
- **Locate scope**: Routes, handlers, services, utils, middleware, db/sqlc packages, migrations, and cross-service dependencies.
- **Read docs**: `BACKEND_CONSISTENCY.md` (CRITICAL standards), folder READMEs (app, routes, handler, services, utils, middleware, db).
- **Trace dependencies**: HTTP flow (route -> handler -> service -> utils -> db/sqlc) and auth/session paths (cookies vs bearer, middleware validation).
- **DB awareness**: Review migrations and `db/.../queries`; plan sqlc regeneration.

## Plan first
- **Design contracts**: Define request/response types using custom structs; align SessionResponse, cookies/headers, primary-device metadata.
- **Auth/session rules**: Plan web (cookies) vs native (bearer) impacts, middleware changes, and expiry/primary-device behavior.
- **Schema impacts**: Decide migration steps and sqlc changes before coding.
- **Architecture compliance**: Ensure handler-service separation, UUID handling, and custom struct responses follow BACKEND_CONSISTENCY.md standards.
- **Endpoint naming source-of-truth**: Treat registered route paths in `routes/*.go` as canonical. Any docs and frontend references must match these exact paths (e.g., `/personal/chat/ack`, `/personal/chat/sync-actions`).

## Execute carefully
- **No ad-hoc SQL**: Edit `db/.../queries` only; regenerate sqlc.
- **CRITICAL: Follow Handler-Service Separation**:
  - Handlers: Only context validation, payload binding, service calls, response handling
  - Services: All business logic, UUID parsing, data validation, response mapping
  - Never put business logic in handlers
- **CRITICAL: UUID Handling Rule**:
  - Database: Store as native `uuid.UUID`
  - Service Layer: Parse from strings, work with `uuid.UUID` internally  
  - Frontend API: Always return UUIDs as strings using `.String()` conversion
- **CRITICAL: Response Struct Rule**:
  - Always return custom structs to frontend, never `map[string]interface{}`
  - Service Layer: Return typed response structs
  - Exception: Only use `map[string]interface{}` for dynamic wrapper objects
- **CRITICAL: Runtime Safety Requirements**:
  - Type assertions MUST use two-value form with `ok` check
  - Always check for nil before dereferencing pointers
  - Validate context extraction with proper type checking
  - Implement panic recovery in goroutines
  - Handle database errors with proper wrapping
- **Update docs**: Refresh READMEs/consistency guide when flows change.

## Post-change sanity
- Re-run auth/session flows across web/native; verify middleware checks and context enrichment.
- Ensure schema/queries match generated code; rerun sqlc if needed.
- Validate cookies/headers and primary-device rules; add tests/logging for new error paths.
- **CRITICAL: Verify Architecture Standards**:
  - Handler-service separation: Ensure no business logic in handlers
  - UUID handling: Verify UUIDs converted to strings in API responses
  - Custom struct responses: Ensure no `map[string]interface{}` in service responses (except wrapper objects)
  - Type safety: Verify all responses use proper model structs
  - **Runtime safety**: Verify all type assertions use `ok` check, pointer dereferences are nil-safe
- **Reference examples**: Compare with `contact_handler.go` and `chat_handler.go` for correct patterns.
