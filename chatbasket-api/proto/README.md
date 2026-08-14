# Protobuf Schema Standards & Conventions

This directory contains the canonical Protocol Buffers (v3) schemas for **ChatBasket Backend (`chatbasket-api`)** and **Frontend (`chatbasket`)**.

---

## 1. Timestamp Schema Standard (`google.protobuf.Timestamp`)

### The `optional` Declaration Rule
Even though Proto3 technically treats all composite messages (including `google.protobuf.Timestamp`) as nullable on the wire, **you MUST explicitly declare `optional` on timestamp fields that are legitimately nullable in business domain logic**.

```protobuf
// ❌ AMBIGUOUS: Unclear if this field is required or nullable in business logic
google.protobuf.Timestamp lastReadAt = 1;

// ✅ CLEAR & EXPLICIT: Clearly documents that the field can be absent/null
optional google.protobuf.Timestamp lastReadAt = 1;

// ✅ CLEAR & EXPLICIT: Mandatory creation timestamp, must always be populated
google.protobuf.Timestamp createdAt = 2;
```

---

### Why Explicit `optional` is Required

1. **Clear Domain Intent & Self-Documentation**:
   Anyone reading the `.proto` file can immediately tell whether a timestamp is guaranteed to exist on every response/event or if it represents an action that may not have occurred yet (e.g. unread, undelivered, no expiration).

2. **Unambiguous Frontend Ingestion**:
   Frontend engineers mapping wire responses to state know precisely which date helper to use:
   * **No `optional`** (`google.protobuf.Timestamp createdAt = 1;`):
     Mapped with **`mapTimestamp(ts)`** (returns non-nullable `string`, throws on missing value as a data integrity bug).
   * **Explicit `optional`** (`optional google.protobuf.Timestamp lastReadAt = 2;`):
     Mapped with **`mapOptionalTimestamp(ts)`** (returns `string | null`).

3. **Prevents Silent Contract Violations**:
   Eliminates ambiguity between backend producers and client consumers about whether a missing timestamp is a valid business state or a backend bug.

---

## 2. Quick Reference: Timestamp Declarations

| Business Meaning | Proto Declaration | Backend Go Type | Frontend TS Helper | Frontend TS Return |
| :--- | :--- | :--- | :--- | :--- |
| **Mandatory** *(Entity/event cannot exist without it)* | `google.protobuf.Timestamp createdAt = 1;` | `*timestamppb.Timestamp` *(Always populated)* | `mapTimestamp(ts)` | `string` |
| **Nullable / Optional** *(Action has not happened or is optional)* | `optional google.protobuf.Timestamp lastReadAt = 2;` | `*timestamppb.Timestamp` *(May be nil)* | `mapOptionalTimestamp(ts)` | `string \| null` |

---

## 3. Code Generation Workflow

Whenever you modify any `.proto` file in this directory:

### Backend (Go)
Run from `chatbasket_backend/chatbasket-api`:
```bash
buf generate
```
* Generates Go structs and ConnectRPC handlers into `gen/proto/`.

### Frontend (TypeScript / React Native)
Run from `chatbasket`:
```bash
buf generate
```
* Generates TypeScript interfaces and schemas into `src/gen/proto/`.

---

## 4. Naming Conventions

* Message names: `PascalCase` (e.g., `Message`, `AcknowledgeDeliverySsePayload`).
* Field names: `camelCase` (e.g., `chatId`, `messageIds`, `deliveredAt`).
