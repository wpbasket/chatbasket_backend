# utils/

Shared helpers for auth/session flows and other cross-cutting logic. Keep services thin by centralizing reusable logic here.

## Key Utilities
- `auth_flow_utils.go`: OTP verification, session creation, primary-device auto-promotion.
- `hashingTextUtils.go`: HMAC + encryption helpers.
- `otpUtils.go`: OTP creation and expiry checks.
- `passwordUtils.go`: password hashing/validation.
- `emailUtils.go`: email dispatch helpers.

## Guidelines
- Use utils for shared business helpers, not for routing or handler logic.
- Avoid duplicating validation logic in services; centralize where appropriate.
- Keep functions pure where possible; minimize side effects.
- Add new cross-cutting helpers here rather than in handlers.

## References
Services should import these helpers rather than re-implementing authentication or hashing flows.
