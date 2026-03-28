# services/

Business logic layer for the backend. Services orchestrate sqlc queries and shared utils, while handlers remain thin. Services **must not** depend on Echo.

## Responsibilities
- Validate inputs and UUIDs
- Execute db operations through sqlc packages
- Map results into typed model structs
- Return `*model.ApiError` on failures

## Global Service
`GlobalService` (see `services/base_service.go`) is the shared service container used by handlers and domain services. It holds:
- `Appwrite` client (standard timeout)
- `AppwriteStorage` client (long timeout for uploads)
- `AuthQueries` + `PersonalQueries` (sqlc)
- `CosmosClient` (Azure Cosmos client)
- `AuthService` reference for shared auth logic

## Appwrite Integration
Two clients are maintained in `appwriteinternal`:
- **Appwrite**: standard client for auth/db tasks (short timeout)
- **AppwriteStorage**: long-timeout client for large file uploads

Use the storage client **only** for large upload operations to avoid slow failures on regular calls.

## Service Rules
- Keep business logic in services, not handlers.
- Do not use ad-hoc SQL; use sqlc queries.
- Return typed response structs (no `map[string]interface{}` except wrapper objects).
- Always convert UUIDs to strings in API responses.
