# utils/

Shared helpers for auth/session flows and other cross-cutting logic.
- `auth_flow_utils.go`: OTP verify (3m expiry), session creation with HMAC, primary-device auto-promotion.
- Hashing, HMAC, expiry helpers live here to keep services thin.

Add new cross-cutting helpers here; avoid putting business logic in handlers.
