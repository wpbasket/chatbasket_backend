# Timestamp & Date Handling Standards

A concise, definitive reference for handling dates and timestamps across **ChatBasket Backend (`chatbasket-api`)** and **Frontend (`chatbasket`)**.

> [!IMPORTANT]
> **Verify Against Active Code**: This document serves as an architectural pattern guide. Never assume docs are absolute ground truth without cross-checking the active codebase (`*.proto`, `util.date.ts`, handler implementations), as contracts and features evolve over time.

---

## 1. Quick Reference Matrix

| Layer | Type | Conversion To Next Layer |
| :--- | :--- | :--- |
| **Database (PostgreSQL)** | `TIMESTAMPTZ` (UTC) | Loaded by SQLC as native Go `time.Time` |
| **Backend Domain (Go)** | `time.Time` | Output to Wire: `timestamppb.New(t)` |
| **Wire Protocol (RPC / SSE)** | `google.protobuf.Timestamp` | Input to Go Domain: `ts.AsTime()` |
| **Frontend Wire Ingestion** | `google.protobuf.Timestamp` | Ingest to State: `mapTimestamp(ts)` |
| **Frontend State & UI (TS)** | ISO-8601 `string` | Display in UI: `formatDateTime(isoStr)` |

---

## 2. Backend Guidelines (Go)

### A. Outgoing: Go `time.Time` ➔ Wire Protobuf Timestamp
Use `timestamppb.New()` for ConnectRPC responses and SSE payloads:

```go
import "google.golang.org/protobuf/types/known/timestamppb"

// Active / non-null time
CreatedAt: timestamppb.New(message.CreatedAt)

// Optional / nullable database time (*time.Time or pgtype.Timestamptz)
if dbTime.Valid {
    CreatedAt: timestamppb.New(dbTime.Time)
}
```

### B. Incoming: Wire Protobuf Timestamp ➔ Native Go `time.Time`
Use `.AsTime()` whenever client RPC requests supply a timestamp that needs to be queried, compared, or stored in PostgreSQL:

```go
// 1. Guard optional/nullable fields to avoid default 1970-01-01 Unix Epoch
var targetTime time.Time
if req.Msg.ScheduledAt != nil {
    targetTime = req.Msg.ScheduledAt.AsTime() // Returns native time.Time in UTC
}

// 2. Use targetTime in Postgres queries or Go time comparisons (time.Since, time.Before)
```

> **Important**: Never pass `*timestamppb.Timestamp` directly to PostgreSQL/`pgx` drivers or Go `time` operators — always convert via `.AsTime()`.

### C. String Format (When Protobuf field is explicitly `string`)
For legacy parity where a field is defined as `string`:

```go
readAtStr := chat.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
```

---

## 3. Frontend Guidelines (TypeScript / React Native)

### A. Converting Protobuf Timestamp to Application State
All modules use the central utilities from `src/utils/commonUtils/util.date.ts`:

```typescript
import { mapTimestamp, mapOptionalTimestamp } from "@/utils/commonUtils/util.date";

// Required Timestamp field -> Returns ISO-8601 string
createdAt: mapTimestamp(protoMsg.createdAt),

// Optional Timestamp field -> Returns ISO-8601 string or null
lastReadAt: mapOptionalTimestamp(protoChat.otherUserLastReadAt),
```

### B. Why `mapTimestamp()` is Universal
- Converts `{ seconds, nanos }` into a standard ISO string: `"2026-08-11T05:45:00.123Z"`.
- Standard ISO strings work seamlessly with `new Date()`, SQLite, MMKV, and UI date formatters (`formatDateTime()`) across all platforms (iOS, Android, Web).

---

## 4. Key Rules

1. **PostgreSQL & Server Time are Authoritative**: Always generate creation/update timestamps on the server (`NOW()` or `time.Now()`) to eliminate client clock skew.
2. **Never use standard `json.Marshal` on `*timestamppb.Timestamp`**: It outputs raw struct JSON `{"seconds": ..., "nanos": ...}`. Use official `proto.Marshal` (for binary SSE) or `protojson.Marshal` (for JSON).
3. **Always use UTC**: All timestamps must be generated, stored, and transmitted in UTC.
