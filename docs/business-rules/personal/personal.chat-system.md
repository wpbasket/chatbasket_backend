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
   - User must have an active primary device (native only)
   - Web sessions cannot send/receive messages without a primary device
5. **Neither user is admin-blocked**
   - `users(id=sender).is_admin_blocked` is FALSE
   - `users(id=recipient).is_admin_blocked` is FALSE

---

## 2) Chat system (primary-device-centric relay)

### 2.1 Core idea
The system operates as a **primary-device-centric relay**. The backend acts as a **temporary bridge**, not a permanent message vault.

- **Native (Primary Device)**: The authoritative source of truth. Each primary native platform (iOS/Android) stores the full, encrypted chat history. It is responsible for acknowledging receipt to the backend to trigger relay purging and for serving P2P sync requests to secondary devices.
- **Secondary Devices (Web/Native)**: Additional sessions that provide a window into the account. They receive real-time updates via WebSockets but rely on P2P sync with the Primary Device for history already purged from the relay.
- **Backend (Relay)**: Messages are stored temporarily and pushed to **all online devices** via WebSockets. The relay row is only purged after explicit acknowledgement from the **Primary devices of both participants**.


### 2.2 Message Delivery & Backend Relay

The system prioritizes real-time delivery while ensuring Primary devices maintain an authoritative history.

#### 2.2.1 Delivery Paths
1.  **WebSocket Push**: When a message is sent, the backend immediately broadcasts it to **all active WebSocket connections** for both the sender and recipient (including all Primary and Secondary devices).
2.  **Relay Fetch**: If a device (Primary or Secondary) was offline during the push, it can fetch missed messages directly from the backend **as long as the message remains in the relay**.
3.  **P2P Sync**: Once a message is purged from the relay, secondary devices can **only** retrieve it by requesting a P2P sync from their respective Primary device via WebRTC.

#### 2.2.2 Backend Purging Rules (Double-Primary Acknowledgement)
The backend relay is governed by strict "Double-Primary Acknowledgement" rules:

- **Storage**: Messages persist in the `messages` table until fully acknowledged.
- **Acknowledgement**:
    - **Any device** can send a delivery ACK to mark the message as "delivered" (updating the double-grey-tick UI).
    - **Only Primary devices** can trigger the `delivered_to_recipient_primary` and `synced_to_sender_primary` flags.
- **Purge Condition**: A message is eligible for hard deletion from the backend relay **only when both** of the following are true:
    1.  `synced_to_sender_primary = TRUE` (Sender's Primary has confirmed receipt).
    2.  `delivered_to_recipient_primary = TRUE` (Recipient's Primary has confirmed receipt).
- **File Messages**: For messages with type `image`, `video`, `audio`, or `file`, the message record contains the file metadata directly. When such a message is purged, the associated files and thumbnails are **permanently deleted** from the backend storage.
- **Consequence**: Once purged, the backend no longer has any copy of the message, its metadata, or its files. New secondary devices or re-installed primaries appearing after this point must rely on P2P sync or backups.


### 2.3 Edge-case policies (defaults)
These are the default policies to implement unless product requirements change:

- **Account creation**
  - Can happen from web or native
  - Sending or receiving messages is **not allowed** until user has an active primary device (native only)

- **Recipient primary offline**
  - Queue on backend until delivery
  - TTL default: **30 days** 

- **Sender primary offline (secondary-device sends)**
  - Deliver to recipient first
  - Queue sender-primary sync
  - TTL default: **30 days**

- **Blocking Policy**
  - **Strict Scope**: These rules apply **only** to the two specific users involved in the block. It does not affect their interactions with other users in the system.
  - **Hard Barrier**: As soon as a block is created, the system immediately stops all new message attempts between these two users. The sender will receive an error.
  - **Automatic Cleanup**: Creating a block automatically deletes the contact relationship in **both** directions for these two users. They are removed from each other's lists instantly.
  - **Relay Purge**: Any messages currently sitting in the backend relay (waiting for delivery) between these two specific users are **physically deleted** immediately.

- **Account deletion**
  - Delete all interconnected data (FK cascade)

- **Storage full on primary device**
  - Retry delivery
  - Notify user
  - TTL default: **7 days** for “storage full” delivery failures

- **Primary switching**
  - Require old primary online
  - If old primary offline: warn data loss; do not switch without explicit confirmation


### 2.4 Chat Status & Receipts

Messages go through 4 visual states. Since the relay deletes messages after both primaries acknowledge them, we store delivery/read timestamps on the `chats` table so the status survives even after messages are purged.


#### 2.4.1 The 4 Tick States (Sender Side Only)
These icons only appear on messages **you sent**:

| State | Icon | What triggers it |
|-------|------|-----------------|
| Pending | 🕒 Clock | Message saved locally, server hasn't confirmed yet |
| Sent | ✅ Single grey tick | Server confirmed the message (any sender device, Primary or Secondary) |
| Delivered | ✅✅ Double grey tick | Any recipient device (Primary or Secondary) received the message |
| Read | ✅✅ Double green tick | Recipient opened the chat on any device (Primary or Secondary) |


#### 2.4.2 How Status is Tracked

**Two separate tracking systems work together:**

**System 1 — Per-message flags** (on the `messages` table):
- `delivered_to_recipient`: Set to `TRUE` when any recipient device confirms receipt.
- `delivered_to_recipient_primary`: Set to `TRUE` only when the recipient's **Primary** device confirms. This flag controls when the server can delete the message from the relay.
- There is no per-message "read" flag.
- **Important**: These flags only exist while the message is in the relay. Once both primaries acknowledge (`delivered_to_recipient_primary = TRUE` AND `synced_to_sender_primary = TRUE`), the message is deleted and these flags are gone.

**System 2 — Chat-level timestamps** (on the `chats` table):
- `p1_last_delivered_at` / `p2_last_delivered_at`: Records the time of the latest delivered message for participant 1 and participant 2 respectively. This timestamp can only move forward, never backward (the SQL uses `GREATEST()`).
- `p1_last_read_at` / `p2_last_read_at`: Set to `now()` whenever participant 1 or participant 2 opens a chat.
- The backend returns the **other** participant's values to the frontend as `other_user_last_delivered_at` and `other_user_last_read_at` (calculated in the `GetUserChats` query).

**Why do we need System 2?** Because messages get deleted from the relay after both primaries acknowledge them. Once deleted, we can no longer check per-message flags. The chat-level timestamps survive forever on the `chats` table and allow the inbox to still show the correct tick icon.


#### 2.4.3 How the Frontend Decides Which Tick to Show

The frontend checks status in this order (first match wins):

1. **Is it Read?** → Compare `message.created_at <= other_user_last_read_at + 10s`. If yes → Double green tick.
2. **Is it Delivered?** → Check if `delivered_to_recipient == TRUE` OR `message.created_at <= other_user_last_delivered_at + 10s`. If yes → Double grey tick.
3. **Otherwise** → Single grey tick (Sent).

The **+10 second buffer** is added to handle small timing differences between devices and the server.

This same logic runs in three places:
- `GetUserChats` query (backend, for the inbox preview — without the 10s buffer)
- `ChatListItem.tsx` (frontend, for the inbox preview — with 10s buffer)
- `[chat_id].tsx` `MessageItemWrapper` (frontend, for each message inside a chat — with 10s buffer)


#### 2.4.4 When Do These Timestamps Get Updated?

**When a message is received** (recipient's device calls `AcknowledgeDelivery`):
- The per-message `delivered_to_recipient` flag is set to `TRUE`.
- The chat's `last_delivered_at` timestamp advances to that message's `created_at` time.
- If the device is Primary: also sets `delivered_to_recipient_primary = TRUE`, and bulk-marks all older text messages as primary-delivered too.

**When a chat is opened** (user calls `MarkChatRead`):
- The user's `unread_count` resets to 0.
- Both `last_read_at` and `last_delivered_at` jump to `now()`.
- If the device is Primary: bulk-sets `delivered_to_recipient_primary = TRUE` for all undelivered messages in that chat. Secondary devices never do this bulk update.

### 2.5 P2P WebRTC Sync Architecture (Secondary ↔ Primary)

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
1. Secondary connects to WebSocket signaling server
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
1. Maintain persistent WebSocket connection to signaling server
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
- `created_at` (TIMESTAMPTZ) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ) — Record last update timestamp

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
Temporary message relay storage. Messages are **ephemeral** — they are deleted after both primary devices acknowledge receipt.

**Columns:**
- `id` (UUID, Primary Key) — Message identifier, Go-generated
- `chat_id` (UUID, NOT NULL) — References chats(id)
- `sender_id` (UUID, NOT NULL) — References users(id)
- `recipient_id` (UUID, NOT NULL) — References users(id)
- `content` (TEXT, NOT NULL, max 5000 chars) — Message content
- `message_type` (TEXT, NOT NULL, DEFAULT 'text') — One of: `text`, `image`, `video`, `audio`, `file`, `unsent`
- `file_id` (TEXT) — File identifier for media messages
- `file_name` (TEXT) — Original file name
- `file_size` (BIGINT) — File size in bytes, max 100MB
- `file_mime_type` (TEXT) — MIME type of the file
- `file_token_id` (TEXT) — File access token ID
- `file_token_secret` (TEXT) — File access token secret
- `file_token_expiry` (TIMESTAMPTZ) — File token expiration
- `thumbnail_file_id` (TEXT) — Thumbnail file ID for media
- `thumbnail_token_id` (TEXT) — Thumbnail access token ID
- `thumbnail_token_secret` (TEXT) — Thumbnail access token secret
- `delivered_to_recipient` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Any recipient device confirmed receipt
- `delivered_to_recipient_primary` (BOOLEAN, DEFAULT FALSE) — Recipient's **Primary** device confirmed receipt (controls relay purge)
- `synced_to_sender_primary` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Sender's Primary device confirmed receipt
- `deleted_by_sender` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Sender deleted this message for themselves
- `deleted_by_recipient` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Recipient deleted this message for themselves
- `delivery_attempts` (INTEGER, NOT NULL, DEFAULT 0) — Number of delivery attempts
- `expires_at` (TIMESTAMPTZ, NOT NULL) — 30-day TTL default
- `created_at` (TIMESTAMPTZ) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ) — Record last update timestamp

**Constraints:**
- `sender_id != recipient_id` (no self-messages)
- File size max 100MB
- Text/unsent messages must have `file_id = NULL`; media messages must have `file_id IS NOT NULL`
- Foreign keys to chats(id) and users(id) with CASCADE delete

**Indexes:**
- Primary key on `id`
- `(recipient_id, delivered_to_recipient, expires_at)` WHERE `delivered_to_recipient = FALSE` — Pending delivery queue
- `(sender_id, synced_to_sender_primary, expires_at)` WHERE `synced_to_sender_primary = FALSE` — Pending sender sync queue
- `(expires_at)` WHERE `expires_at IS NOT NULL` — TTL cleanup job
- `(chat_id, created_at DESC)` — Chat history retrieval
- `(file_id)` WHERE `file_id IS NOT NULL` — File cleanup
- `(file_token_expiry)` WHERE `file_id IS NOT NULL AND file_token_expiry IS NOT NULL` — Expired file tokens

**Triggers:**
- `messages_timestamps_trigger` — Automatic timestamp management via `set_timestamps()`

---

#### A.1.3 message_sync_actions
Relay for cross-device synchronization of unsend and delete-for-me actions. These records are queued for the Primary device to pick up.

**Columns:**
- `id` (UUID, Primary Key) — Sync action identifier
- `user_id` (UUID, NOT NULL) — The user whose Primary device should receive this action, references users(id)
- `action_type` (TEXT, NOT NULL) — One of: `unsend`, `delete_for_me`
- `payload` (JSONB, NOT NULL) — Action-specific data (contains message_ids, chat_id, etc.)
- `delivered_to_primary` (BOOLEAN, NOT NULL, DEFAULT FALSE) — Whether the Primary device has acknowledged this action
- `created_at` (TIMESTAMPTZ) — Record creation timestamp
- `updated_at` (TIMESTAMPTZ) — Record last update timestamp

**Constraints:**
- `action_type` must be one of: `unsend`, `delete_for_me`
- Foreign key to users(id) with CASCADE delete

**Indexes:**
- Primary key on `id`
- `(user_id, delivered_to_primary, created_at)` — Fetching pending actions for a user

**Triggers:**
- `sync_actions_timestamps_trigger` — Automatic timestamp management via `set_timestamps()`