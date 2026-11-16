# Go DB Testing — pgxmock + Testing DB

Future tests should use one of these patterns.

## 1. Unit tests: use `pgxmock/v5` (preferred)

Use when testing service/query behavior without real Postgres.

Dependency:

```bash
go get github.com/pashagolub/pgxmock/v5
```

Basic pattern:

```go
mock, err := pgxmock.NewPool()
if err != nil {
    t.Fatal(err)
}
defer mock.Close()

mock.ExpectQuery("SELECT (.+) FROM users").
    WithArgs(userID).
    WillReturnRows(pgxmock.NewRows([]string{"e2ee_public_key"}).AddRow(publicKey))

// call code using mock as sqlc DBTX / pgx pool interface

if err := mock.ExpectationsWereMet(); err != nil {
    t.Fatalf("unmet pgxmock expectations: %v", err)
}
```

Notes:
- Default query matcher is regexp; keep expected SQL loose: `"SELECT (.+) FROM users"`, `"INSERT INTO\\s+messages"`.
- Use `WithArgs(...)` for critical args.
- Use `pgxmock.AnyArg()` for generated UUID/time args.
- Use `pgxmock.NewRows([]string{...}).AddRow(...)` for query results.
- Always call `ExpectationsWereMet()`.
- **Limitation with Concrete `*pgxpool.Pool`**:
  - `pgxmock` implements a `PgxPoolIface` interface, which cannot be cast or assigned to fields of concrete type `*pgxpool.Pool`.
  - To respect production types, we use a **Split Testing Strategy**:
    - **Unit Tests**: Test non-transaction methods (and methods that guard `s.Pool != nil`, such as `SendMessage`, `MarkChatRead`, `AcknowledgeAndReadBatch`, `UnsendMessageHandler`, `DeleteMessageForMe`) using `pgxmock` with `Pool: nil`.
    - **Integration Tests**: Test methods that require `Pool.Begin(ctx)` (e.g., `ConfirmChatUpload`, `deleteMessageFromRelay`, real tx rollback/locking) against a real testing database pool.

## 2. Integration tests: use testing DB URL

Use only when behavior depends on real Postgres semantics.

Env var priority:

```go
dsn := os.Getenv("DatabaseURLTesting")
if dsn == "" {
    dsn = os.Getenv("DATABASE_URL_PG_TESTING")
}
if dsn == "" {
    t.Skip("DatabaseURLTesting/DATABASE_URL_PG_TESTING not set")
}
```

Connect:

```go
pool, err := pgxpool.New(context.Background(), dsn)
if err != nil {
    t.Fatalf("connect testing db: %v", err)
}
t.Cleanup(pool.Close)
```

When possible, avoid mutating real tables. Prefer temp tables:

```go
_, err := pool.Exec(ctx, `
    CREATE TEMP TABLE users (
        id UUID PRIMARY KEY,
        e2ee_public_key TEXT
    ) ON COMMIT PRESERVE ROWS
`)
if err != nil {
    t.Fatalf("create temp table: %v", err)
}
```

Notes:
- Use `DatabaseURLTesting` / `DATABASE_URL_PG_TESTING`, never production DB.
- Prefer temp tables for isolated tests.
- If real tables must be used, create unique UUID rows and clean with `t.Cleanup`.
- Keep integration tests small; pgxmock unit tests should cover most service behavior.

## Existing reference

See:

```text
chatbasket-api/internal/modules/core/core_auth/core_auth_svc_e2ee_test.go
chatbasket-api/internal/modules/personal/personal_chat/personal_chat_svc_revision_test.go
```

Covers:
- `pgxmock.NewPool()` setup
- sqlc query mocking
- `WithArgs`
- `WillReturnRows`
- `ExpectationsWereMet`
- E2EE session keys, `keys_revision`, and key-revision staleness checks
