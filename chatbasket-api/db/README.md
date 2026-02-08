# db/

SQL definitions and sqlc configuration.
- `queries/`: SQL files for sqlc generation.
- `migrations/`: Schemas for sqlc and migrations.
- `sqlc.yaml`: Configures package output under `internal/db/...`.

Add/modify SQL here and regenerate sqlc. Do not embed SQL in handlers/services.
