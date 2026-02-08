# middleware/

Shared Echo middleware. Cross-cutting concerns live here.

## AuthSessionMiddleware
- **Dual platform auth**: Accepts native bearer `Authorization: Bearer <sessionId>:<userId>` and web HttpOnly cookies (`sessionId`, `userId`).
- **HMAC validation**: Recomputes HMAC of sessionId with `AuthSecret`, checks DB via `CheckSessionIsValid` and user existence.
- **Context enrichment**: Sets `uuidUserId`, `userId`, `sessionId`, `platform`, `email` on Echo context for downstream handlers.
- **Errors**: Returns typed `SessionError` with `missing_auth`, `invalid_user_id`, `session_invalid`, or `user_not_found`.
