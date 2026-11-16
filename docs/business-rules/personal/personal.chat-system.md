## 1) Messaging eligibility rules (who can send to whom)

A sender **can send a message** to a recipient only if all are true:

1. **Recipient is in sender’s contacts**
   - Exists `user_contacts(owner_user_id=sender, contact_user_id=recipient)`
2. **Recipient is not private**
   - Private profiles cannot be added as contacts anyway
3. **No blocks in either direction**
   - `user_blocks(sender -> recipient)` does not exist
   - `user_blocks(recipient -> sender)` does not exist
4. **Primary device rules satisfied**
   - Both the sender and recipient must have an active primary device (`is_central = TRUE` and unexpired).
   - Intended policy is native-only (enforced on manual switching via `SetCentralDevice`, which rejects `web`); **temporarily**, `CreateSessionFlow` auto-promotes the first session on any platform (including `web`) to primary when no primary exists so web messaging works without a native device.
5. **Neither user is admin-blocked**
   - `users(id=sender).is_admin_blocked` is FALSE
   - `users(id=recipient).is_admin_blocked` is FALSE

---

## 2) Chat system (primary-device-centric relay)

### 2.1 Core idea
The system operates as a **primary-device-centric relay**. The backend acts as a **temporary bridge**, not a permanent message vault.

- **Native (Primary Device)**: The authoritative source of truth. Each primary native platform (iOS/Android) stores the full, encrypted chat history. It is responsible for acknowledging receipt to the backend to trigger relay purging and for serving Encrypted History Sync (`history_sync`) requests to secondary devices.
- **Secondary Devices (Web/Native)**: Additional sessions that provide a window into the account. They receive real-time updates via ConnectSSE streams and rely on Encrypted History Sync (`history_sync`) with the Primary Device for history already purged from the relay.
- **Backend (Relay)**: Messages are stored temporarily and pushed to **all online devices** via ConnectSSE. Payloads and attached R2 files are stripped once the **Primary devices of both participants** acknowledge delivery/sync; the lightweight row is hard-deleted once read receipts are also acknowledged by both sides (or upon TTL expiry, unsend, or block).


### 2.2 Message Delivery & Backend Relay

The system prioritizes real-time delivery while ensuring Primary devices maintain an authoritative history.

#### 2.2.1 Delivery Paths
1.  **ConnectSSE Push**: When a message is sent, the backend immediately broadcasts it to **all active SSE stream connections** for both the sender and recipient (including all Primary and Secondary devices).
2.  **Relay Fetch (`/pending`)**: If a device (Primary or Secondary) was offline during the push, it can fetch missed messages directly from the backend **as long as the message remains in the relay**. Both Primary and Secondary devices fetch all available relay messages for their user account.
3.  **Client ACK Boundaries**: The client application manages `isPrimary` (`isCentral`) on launch. Primary devices issue sender-sync ACKs (`synced_to_sender_primary`) and primary-recipient ACKs; Secondary devices consume and display relay messages without issuing primary-only ACKs.
4.  **Encrypted History Sync (`history_sync`)**: Once a message is purged from the relay, secondary devices retrieve historical messages from their Primary device via server-staged E2EE `history_sync` (`RequestHistorySync` → `FetchHistorySync` → `UploadHistorySync` → `DownloadHistorySync` → `AcknowledgeHistorySync`; 10-minute TTL, up to 90 MB ciphertext). Direct WebRTC P2P sync is **[Future]**.

#### 2.2.2 Backend Purging Rules (Two-Stage Relay Lifecycle)
The backend relay follows a two-stage purge lifecycle governed by Double-Primary Acknowledgement and read-receipt completion:

- **Storage**: Messages persist in the `messages` table until purged (or expired after `DefaultMessageTTL` = 30 days). A companion partitioned tombstone table (`message_history`) retains `(id, created_at)` for 33 days to reject duplicate re-sends after a row is purged.
- **Acknowledgement**:
    - **Any recipient device** can send a delivery ACK to set `delivered_to_recipient = TRUE`.
    - **Only Primary devices** can set `delivered_to_recipient_primary = TRUE` and `synced_to_sender_primary = TRUE`, or send `AcknowledgeReadReceiptBatch` (`read_acked_by_sender = TRUE`).
- **Stage 1 — Payload & R2 File Strip (Double-Primary Delivery/Sync)**: Once both `synced_to_sender_primary = TRUE` and `delivered_to_recipient_primary = TRUE` are true (and read receipts are not yet complete), the server keeps the row for read-receipt tracking but strips the payload (`StripDeliveredMessagePayload`: clears `content` and nulls `file_*`/`thumbnail_*` columns) and deletes the attached R2 file (`pending_uploads` registration + async R2 delete).
- **Stage 2 — Hard Deletion**: A message row is hard-deleted from `messages` (`deleteMessageFromRelay` / `CleanupFullyAcknowledgedReadMessagesInChat` / `CleanupDatabaseOnly`) only when:
    1. `synced_to_sender_primary = TRUE`
    2. `delivered_to_recipient_primary = TRUE`
    3. `read_by_recipient = TRUE`
    4. `read_acked_by_sender = TRUE`
    5. `file_id IS NULL` (R2 object already stripped/deleted)
    *(Rows are also hard-deleted on TTL expiry, block cleanup, or account deletion.)*
- **File Messages**: For messages with type `image`, `video`, `audio`, or `file`, the encrypted message `content` envelope carries original file metadata (`fileName`, `fileSize`, `fileMimeType`) — the backend stores those columns as `NULL` and only tracks the R2 `file_id`. There is no separate backend thumbnail upload or deletion.
- **Consequence**: Once Stage 1 strips the payload/file (and after Stage 2 deletes the row), the backend no longer holds the ciphertext or R2 object. Secondary devices or newly linked primaries must retrieve prior history via Encrypted History Sync (`history_sync`) or local backups.


### 2.3 Edge-case policies (defaults)
These are the default policies to implement unless product requirements change:

- **Account creation**
  - Can happen from web or native
  - Sending or receiving messages is **not allowed** until the user has an active primary device (intended: native-only; **temporarily**, `CreateSessionFlow` auto-promotes the first session on any platform, including `web`, when no primary exists)

- **Recipient primary offline**
  - Queue on backend until delivery
  - TTL default: **30 days** 

- **Sender primary offline (secondary-device sends)**
  - Deliver to recipient first
  - Queue sender-primary sync
  - TTL default: **30 days**

- **Blocking Policy**
  - **Strict Scope**: These rules apply **only** to the two specific users involved in the block. It does not affect their interactions with other users in the system.
  - **Hard Barrier**: As soon as a block is created, the system immediately stops all new message attempts between these two users. The sender will receive a generic error (`user_blocked` / `action_not_permitted`) without leaking block direction.
  - **Relationship Preservation (Hard Exclusion)**: Creating a block does **not** delete `user_contacts` or `contact_requests`. Relationships are preserved in the DB and hidden at query time (`GetContactableProfilesForViewer`, `GetContactableUserIDs`), making them self-healing upon unblock (see `personal.profile&contact-system.md` §5.2).
  - **Relay Purge**: Creating a block fires `cleanup_sync_actions_on_block` (DB trigger) and `CleanupChatMessagesForBlockAsync` (async deletion of the shared chat's `messages` rows and R2 files), backed by the hourly `CleanupExpiredMessages` and `CleanupDatabaseOnly` background workers.

- **Account deletion**
  - Delete all interconnected data (FK cascade)

- **Storage full on primary device**
  - Retry delivery (all relay messages use `DefaultMessageTTL` = **30 days**)

- **Primary switching**
  - Require old primary online
  - If old primary offline: warn data loss; do not switch without explicit confirmation


### 2.4 Chat Status & Receipts

Messages go through 4 visual states tracked per message. Because the relay retains stripped message rows until read receipts are acknowledged by both sides, discrete delivery and read flags survive Stage 1 payload stripping and drive real-time SSE and catch-up sync.

#### 2.4.1 The 4 Tick States (Sender Side Only)
These icons only appear on messages **you sent**:

| State | Icon | What triggers it |
|-------|------|-----------------|
| Pending | 🕒 Clock | Message saved locally, server hasn't confirmed yet |
| Sent | ✅ Single grey tick | Server confirmed the message (any sender device, Primary or Secondary) |
| Delivered | ✅✅ Double grey tick | Any recipient device (Primary or Secondary) acknowledged delivery (`delivered_to_recipient = TRUE`) |
| Read | ✅✅ Double green/primary tick | Recipient opened the chat and marked the message read (`read_by_recipient = TRUE`) |

#### 2.4.2 How Status is Tracked

**Per-message flags** (on the `messages` table):
- `delivered_to_recipient`: Set to `TRUE` when any recipient device confirms receipt.
- `delivered_to_recipient_primary`: Set to `TRUE` only when the recipient's **Primary** device confirms receipt (required for Stage 1 payload stripping).
- `synced_to_sender_primary`: Set to `TRUE` when the sender's **Primary** device confirms receipt (or at send time if sent from the Primary device).
- `read_by_recipient` & `read_at`: Set to `TRUE` and `now()` when any recipient device reads the message (`MarkChatRead` / `AcknowledgeAndReadBatch`). Requires `delivered_to_recipient = TRUE` (`chk_messages_read_prerequisite`).
- `read_acked_by_sender`: Set to `TRUE` when the sender's **Primary** device confirms receipt of the read receipt (`AcknowledgeReadReceiptBatch`). Requires `read_by_recipient = TRUE` (`chk_messages_read_ack_prerequisite`), and enables Stage 2 hard deletion.

**Chat-level metadata** (on the `chats` table):
- `p1_last_delivered_at` / `p2_last_delivered_at`: High-water mark of the latest delivered message `created_at` per participant (advanced via `GREATEST()`).
- `p1_last_read_at` / `p2_last_read_at`: Timestamp of the participant's latest `MarkChatRead` / `AcknowledgeAndReadBatch` (`now()`).
- `p1_unread_count` / `p2_unread_count`: Incremented on send, decremented on unsend, reset to `0` on read.
- `GetMessagesResponse` returns `otherUserLastReadAt` and `otherUserLastDeliveredAt`; `GetUserChatsResponse` (`rpc_personal_chat.v1.Chat`) returns only chat identity and participant profile fields (not status timestamps, unread counts, or previews).

#### 2.4.3 How the Frontend Decides Which Tick to Show
`MessageBubble` and `ChatListItem` derive outgoing ticks via `deriveOutgoingTick` directly from the message's own persisted flags (`readByRecipient`, `deliveredToRecipient`, and server ACK status), without timestamp-watermark guessing.

#### 2.4.4 When Flags & Timestamps Get Updated
- **`AcknowledgeDelivery` / `AcknowledgeDeliveryBatch`** (recipient): Marks only the requested `messageIds` with `delivered_to_recipient = TRUE` (and `delivered_to_recipient_primary = TRUE` if Primary), advances `last_delivered_at` via `UpdateChatLastDeliveredAt`, and emits `AcknowledgeDeliverySseEvent` to the sender.
- **`AcknowledgeDelivery` / `AcknowledgeDeliveryBatch`** (sender Primary): Marks only the requested `messageIds` with `synced_to_sender_primary = TRUE`.
- **`MarkChatRead`** (recipient): Requires messages to already have `delivered_to_recipient = TRUE`. Resets `unread_count = 0` and sets `last_read_at = now()` (`ResetChatReadStatus`), marks the requested `messageIds` with `read_by_recipient = TRUE` and `read_at = now()`, and emits `MarkChatReadSseEvent` to the sender. Does not modify `last_delivered_at` or `delivered_to_recipient_primary`.
- **`AcknowledgeAndReadBatch`** (recipient): Atomically performs delivery ACK (`UpdateChatLastDeliveredAt` + delivery flags) and read marking (`ResetChatReadStatus` + `MarkMessagesAsRead`) in a single transaction, then emits `MarkChatReadSseEvent` to the sender.
- **`AcknowledgeReadReceiptBatch`** (sender Primary): Marks `read_acked_by_sender = TRUE` on the requested `messageIds` and triggers Stage 2 relay cleanup.

### 2.5 P2P WebRTC Sync Architecture (Secondary ↔ Primary) [Future — Not Implemented]

> **Status: Future / Not Implemented.** Current production uses server-staged E2EE `history_sync` (`011_history_sync.up.sql`, `history_sync_svc.go`). The WebRTC architecture below is preserved as a future design.

#### 2.5.1 Design rationale
To reduce backend bandwidth and latency, secondary devices (web/other native) synchronize **directly with the primary device** using WebRTC peer-to-peer connections.

**Authorization flow:**
- Backend only handles WebRTC signaling (offer/answer/ICE) and authentication
- Actual sync data transfers via P2P data channel (bypasses backend)
- Backend validates session tokens before allowing P2P signaling

**Strict P2P requirement:**
- If P2P connection fails (NAT/firewall issues), there is **no fallback**.
- The user must be shown an error: "Connection cannot be established. Please try using the same network on both devices."

#### 2.5.2 Connection establishment flow

**Secondary device initiates:**
```
1. Secondary connects to signaling stream / initiates request
2. Backend authenticates session
3. Backend checks if user has active primary device
4. If primary online:
   - Backend facilitates WebRTC handshake
   - Secondary creates offer → Backend relays to primary
   - Primary creates answer → Backend relays to secondary
   - ICE candidates exchanged via backend
   - Direct P2P data channel established
...
```

**Primary device responsibilities:**
```
1. Maintain persistent connection / stream to receive signaling
2. Listen for WebRTC offers from secondary devices
3. Respond with WebRTC answers
4. Maintain P2P data channels with all connected secondaries
5. Serve sync requests directly via data channels
```


#### 2.5.3 Security model

**Transport encryption:**
- WebRTC data channels use DTLS (Datagram Transport Layer Security) automatically
- All P2P data is encrypted end-to-end between devices
- Backend cannot decrypt P2P sync data

**Authentication:**
- Backend validates JWT tokens before allowing signaling
- Primary device verifies session ownership before accepting P2P connections
- Secondary devices must prove ownership of same user account

#### 2.5.4 Connection Failure Handling

If P2P connection fails (NAT/firewall blocking):

**Detection:**
- WebRTC connection timeout (30 seconds)
- ICE connection state: "failed" or "disconnected"

**Failure flow:**
- Determine that the connection could not be established.
- Abort sync attempt.
- Display error to user: **"Connection cannot be established in this network. Try using the same network in both devices."**


#### 2.5.5 Developer invariants

- P2P sync is **strictly required**, there is no backend relay fallback.
- Primary device SQLite is **always authoritative**
- Signaling server must validate both peers before facilitating P2P


### 2.6 Media File Handling (SaveToDevice + Web Browser API)

#### 2.6.1 Design philosophy

Media files follow a **two-step user-controlled workflow**:

1. **Step 1: Receive/download to private storage** (automatic)
   - Native: `expo-file-system` → `Paths.document` (private app folder)
   - Web: Synced via P2P → IndexedDB cache (temporary)

2. **Step 2: Save to public storage** (user-initiated)
   - Native Android: `SaveToDevice` module → DCIM/Downloads (public)
   - Native iOS: Share sheet / Photos library
   - Web: Browser download → user's Downloads folder

**Why two steps:**
- Privacy: Media stays private until user explicitly saves
- Storage control: User decides what to keep publicly


#### 2.6.2 Developer invariants

- **Two-Step Media Workflow**: Media files must be downloaded to private app-only storage first. They are only saved to the public device storage (Gallery/Photos) if the user explicitly triggers a "Save" action.
- SaveToDevice is Android-only (iOS uses share sheet, web uses browser API)
- Public saves are user-initiated only (never automatic)
- **Persistence**: Primary devices store permanently (SQLite + expo-file-system). Secondary Native devices store permanently (SQLite + expo-file-system). Secondary Web devices store in IndexedDB. All data persists until **logout, manual delete from within the app, or manual app data clear from device settings**. No TTL on any device.

---


## Appendix: Database Schema

### A.1 Tables

#### A.1.1 chats
Stores 1v1 chat metadata. This table is **never deleted** (unlike messages) and serves as the permanent record for delivery/read status.

**Columns:**
- `id` (UUID, Primary Key) — Chat identifier, Go-generated
- `participant_1_id` (UUID, NOT NULL) — First participant, references users(id)
- `participant_2_id` (UUID, NOT NULL) — Second participant, references users(id)
- `p1_unread_count` (INTEGER, NOT NULL, DEFAULT 0) — Unread count for participant 1
- `p2_unread_count` (INTEGER, NOT NULL, DEFAULT 0) — Unread count for participant 2
- `p1_last_read_at` (TIMESTAMPTZ) — Last time participant 1 opened the chat
- `p2_last_read_at` (TIMESTAMPTZ) — Last time participant 2 opened the chat
- `p1_last_delivered_at` (TIMESTAMPTZ) — Timestamp of the last message delivered to participant 1
- `p2_last_delivered_at` (TIMESTAMPTZ) — Timestamp of the last message delivered to participant 2
- `last_message_created_at` (TIMESTAMPTZ) — When the last message was created
- `last_message_sender_id` (UUID) — Who sent the last message
- `last_message_id` (UUID) — UUID of the message currently displayed as preview
- `p1_last_message_content` (TEXT) — Last message preview for participant 1
- `p2_last_message_content` (TEXT) — Last message preview for participant 2
- `p1_last_message_type` (TEXT) — Last message type for participant 1 preview
- `p2_last_message_type` (TEXT) — Last message type for participant 2 preview
- `created_at` (TIMESTAMPTZ, NOT NULL) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ, NOT NULL) — Record last update timestamp

**Constraints:**
- Unique pair: `(participant_1_id, participant_2_id)`
- No self-chat: `participant_1_id != participant_2_id`
- Ordered pair: `participant_1_id < participant_2_id`
- Foreign keys to users(id) with CASCADE delete

**Indexes:**
- Primary key on `id`
- `(participant_1_id, created_at DESC)` — For participant 1 chat list
- `(participant_2_id, created_at DESC)` — For participant 2 chat list

**Triggers:**
- `chats_timestamps_trigger` — Automatic timestamp management via `set_timestamps()`

---

#### A.1.2 messages
Temporary message relay storage. Messages are **ephemeral** — payloads and R2 files are stripped after double-primary delivery/sync (Stage 1), and rows are hard-deleted after read receipts are acknowledged by both sides (Stage 2) or on TTL expiry / unsend / block.

**Columns:**
- `id` (UUID, Primary Key) — Message identifier, client-generated
- `chat_id` (UUID, NOT NULL) — References chats(id)
- `sender_id` (UUID, NOT NULL) — References users(id)
- `recipient_id` (UUID, NOT NULL) — References users(id)
- `sender_session_id` (UUID, NOT NULL) — Sender session that authored the message (migration `013`)
- `content` (TEXT, NOT NULL) — E2EE ciphertext (`MaxMessageContentLength = 25000` enforced in Go)
- `message_type` (TEXT, NOT NULL, DEFAULT 'text') — One of: `text`, `image`, `video`, `audio`, `file`, `unsent`
- `reply_to_message_id` (UUID) — Optional referenced message UUID
- `file_id` (TEXT) — R2 prefixed file identifier for media messages (nullable)
- `file_name` (TEXT) — Legacy/unused for E2EE uploads (`NULL`)
- `file_size` (BIGINT) — Legacy/unused for E2EE uploads (`NULL`), DB check max 100MB (`104857600`)
- `file_mime_type` (TEXT) — Legacy/unused for E2EE uploads (`NULL`)
- `file_token_id` (TEXT) — Legacy/unused (`NULL`)
- `file_token_secret` (TEXT) — Legacy/unused (`NULL`)
- `file_token_expiry` (TIMESTAMPTZ) — Legacy/unused (`NULL`)
- `thumbnail_file_id` (TEXT) — Legacy/unused (`NULL`)
- `thumbnail_token_id` (TEXT) — Legacy/unused (`NULL`)
- `thumbnail_token_secret` (TEXT) — Legacy/unused (`NULL`)
- `delivered_to_recipient` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Any recipient device confirmed receipt
- `delivered_to_recipient_primary` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Recipient's **Primary** device confirmed receipt
- `synced_to_sender_primary` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Sender's **Primary** device confirmed receipt
- `deleted_by_sender` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Sender deleted this message for themselves
- `deleted_by_recipient` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Recipient deleted this message for themselves
- `read_by_recipient` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Recipient marked the message read
- `read_acked_by_sender` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Sender's Primary acknowledged the read receipt
- `read_at` (TIMESTAMPTZ) — When recipient read the message
- `delivery_attempts` (INTEGER, NOT NULL, DEFAULT 0) — Delivery attempt counter
- `expires_at` (TIMESTAMPTZ, NOT NULL) — 30-day TTL default (`DefaultMessageTTL`)
- `created_at` (TIMESTAMPTZ, NOT NULL) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ, NOT NULL) — Record last update timestamp

**Constraints:**
- `sender_id != recipient_id` (`messages_valid_participants`)
- `file_size IS NULL OR file_size <= 104857600` (`messages_file_size_limit`)
- `messages_file_type_validation`: `'text'` requires `file_id IS NULL`; `'unsent'` allows any `file_id`; media types (`image`, `video`, `audio`, `file`) require `file_id IS NOT NULL` OR `(delivered_to_recipient_primary = TRUE AND synced_to_sender_primary = TRUE)`
- `chk_messages_read_prerequisite`: `read_by_recipient = FALSE OR delivered_to_recipient = TRUE`
- `chk_messages_read_ack_prerequisite`: `read_acked_by_sender = FALSE OR read_by_recipient = TRUE`
- Foreign keys to `chats(id)` and `users(id)` with `ON DELETE CASCADE`

**Indexes:**
- Primary key on `id`
- `idx_messages_pending_delivery` (`recipient_id, delivered_to_recipient, expires_at`) WHERE `delivered_to_recipient = FALSE`
- `idx_messages_pending_sender_sync` (`sender_id, synced_to_sender_primary, expires_at`) WHERE `synced_to_sender_primary = FALSE`
- `idx_messages_expired` (`expires_at`) WHERE `expires_at IS NOT NULL`
- `idx_messages_chat_history` (`chat_id, created_at DESC`)
- `idx_messages_file_cleanup` (`file_id`) WHERE `file_id IS NOT NULL`
- `idx_messages_expired_file_tokens` (`file_token_expiry`) WHERE `file_id IS NOT NULL AND file_token_expiry IS NOT NULL`
- `idx_messages_files_sender_id` (`sender_id, id`) WHERE `file_id IS NOT NULL`
- `idx_messages_files_recipient_id` (`recipient_id, id`) WHERE `file_id IS NOT NULL`
- `idx_messages_expired_no_file` (`expires_at`) WHERE `file_id IS NULL`
- `idx_messages_ack_no_file` (`created_at`) WHERE `file_id IS NULL AND delivered_to_recipient_primary = TRUE AND synced_to_sender_primary = TRUE AND read_by_recipient = TRUE AND read_acked_by_sender = TRUE`
- `idx_messages_chat_no_file` (`chat_id`) WHERE `file_id IS NULL`
- `idx_messages_unread` (`chat_id, recipient_id, read_by_recipient`) WHERE `read_by_recipient = FALSE`
- `idx_messages_read_unacked` (`chat_id, sender_id, read_by_recipient, read_acked_by_sender`) WHERE `read_by_recipient = TRUE AND read_acked_by_sender = FALSE`
- `idx_messages_recipient_keyset` (`recipient_id, created_at ASC, id ASC`) WHERE `deleted_by_recipient = FALSE`
- `idx_messages_sender_keyset` (`sender_id, created_at ASC, id ASC`) WHERE `deleted_by_sender = FALSE`
- `idx_messages_chat_keyset` (`chat_id, created_at ASC, id ASC`)

**Triggers:**
- `messages_timestamps_trigger` — Automatic timestamp management via `set_timestamps()`

---

#### A.1.3 message_sync_actions
Relay for cross-device synchronization of unsend and delete-for-me actions. These records are queued for the Primary device to pick up.

**Columns:**
- `id` (UUID, Primary Key) — Sync action identifier
- `user_id` (UUID, NOT NULL) — The user whose Primary device should receive this action, references users(id)
- `action_type` (TEXT, NOT NULL) — One of: `unsend`, `delete_for_me`
- `payload` (JSONB, NOT NULL) — Action-specific data (`messageIds`, and `chatId` for `unsend`)
- `delivered_to_primary` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Whether the Primary device has acknowledged this action
- `created_at` (TIMESTAMPTZ, NOT NULL) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ, NOT NULL) — Record last update timestamp

**Constraints:**
- `action_type` must be one of: `unsend`, `delete_for_me`
- Foreign key to `users(id)` with `ON DELETE CASCADE`

**Indexes:**
- Primary key on `id`
- `idx_sync_actions_user_pending` (`user_id, delivered_to_primary, created_at`) — Fetching pending actions for a user
- `idx_sync_actions_created_at` (`created_at`) — 30-day retention cleanup
- `idx_sync_actions_payload_chat_id` (`((payload->>'chatId')::uuid)`) — Expression index for blocked-user cleanup

**Triggers:**
- `sync_actions_timestamps_trigger` — Automatic timestamp management via `set_timestamps()`

---

#### A.1.4 message_history (migration `014`)
Append-only tombstone table of message IDs (`PRIMARY KEY (id, created_at)`), range-partitioned daily on `created_at`. Prevents duplicate re-inserts after a message row is purged from `messages`. Daily partitions (`-1d..+7d`) and drops older than 33 days are managed by `personal_chat_svc_partitions.go`.

---

#### A.1.5 history_sync (migration `011`)
Tracks server-staged E2EE history synchronization requests between a secondary device and the primary device (`id`, `user_id`, `session_id` unique, `chats_json` JSONB, `payload` JSONB nullable, `expires_at` 10-minute TTL).

---

#### A.1.6 pending_uploads (migration `010`)
Tracks presigned R2 uploads awaiting confirmation and expired/deleted R2 files awaiting cleanup (`file_id` PK, `bucket_name`, `r2_key`, `expires_at` 2-hour TTL for presigns or born-expired for deletions).

---

#### A.1.7 personal_sse_outbox (migration `012`)
Cluster fanout outbox for SSE events exceeding the inline `pg_notify` payload cutoff (`id`, `command`, `target_user_id`, `target_session_id`, `exclude_session_id`, `payload` BYTEA, `created_at`, `expires_at` 3-minute TTL).