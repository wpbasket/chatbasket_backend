# db/

SQL definitions and sqlc configuration for the backend.

## Structure
- `queries/`: SQL files consumed by sqlc.
- `migrations/`: Schema migrations.
- `sqlc.yaml`: Configures output packages under `internal/db/...`.

## Rules
- Add or modify SQL only inside `queries/` and `migrations/`.
- Regenerate sqlc after query changes.
- Do not embed SQL directly in services or handlers.
