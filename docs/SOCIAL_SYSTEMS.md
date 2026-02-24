# Social Systems Architecture (Profiles, Contacts, Restrictions, Blocking, Chat)

This document is the **single source of truth** for the interconnected social features in the Chatbasket backend:

- Profile system (`public`, `personal`, `private`)
- One-way contacts + contact requests
- Profile privacy restrictions + exemptions
- Blocking rules (and their cascade effects)
- Phase 6 chat relay system (primary-device-centric + ephemeral server storage) and finalized edge-case policies

It is written to keep future development consistent across DB, sqlc queries, services, and API routes.

---

## 1) System map (where the authoritative logic lives)

### Backend layering
Request flow is:

`routes -> handler -> service -> db/sqlc` (+ `utils` for shared helpers)

### Key folders/files
- **Routes**
  - `chatbasket-api/routes/personal_routes.go`
  - `chatbasket-api/routes/public_routes.go`
- **Contacts**
  - `chatbasket-api/personal/personalhandler/contact_handler.go`
  - `chatbasket-api/personal/personalservice/contact_service.go`
  - `chatbasket-api/db/personal/migrations/002_personal_user_contacts.up.sql`
  - `chatbasket-api/db/personal/migrations/006_personal_contact_requests.up.sql`
  - `chatbasket-api/db/personal/queries/personal_contacts.sql`
- **Blocking**
  - `chatbasket-api/db/personal/migrations/004_personal_user_blocks.up.sql`
  - `chatbasket-api/db/personal/queries/personal_contacts.sql` (block helper query)
- **Profile types + usernames**
  - `chatbasket-api/db/personal/migrations/001_personal_init.up.sql`
  - `chatbasket-api/db/personal/queries/personal_user.sql`
  - `chatbasket-api/personal/personalservice/profile_service.go`
  - `chatbasket-api/utils/hashingTextUtils.go` (HMAC + ChaCha20-Poly1305 username encryption)
- **Restrictions + exemptions**
  - `chatbasket-api/db/personal/migrations/003_personal_user_restrictions.up.sql`
  - `chatbasket-api/db/personal/migrations/005_personal_global_restrictions.up.sql`
  - `chatbasket-api/db/personal/queries/personal_contacts.sql` (returns raw flags)
  - `chatbasket-api/personal/personalservice/contact_service.go` (applies avatar visibility)
- **Primary device (central session)**
  - `chatbasket-api/db/auth/migrations/001_auth_init.up.sql` (`sessions.is_central`)
  - `chatbasket-api/personal/personalservice/setting_service.go` (`SetCentralDevice`)
  - `chatbasket-api/personal/personalhandler/setting_handler.go`

---

## 2) Profile system (public / personal / private)

### 2.1 Profile types
Profile type is stored in Postgres in the **personal** user profile table (`users.profile_type`).

- **`public`**
  - Discoverable
  - Can be added as a contact directly
- **`personal`**
  - Discoverable
  - Requires a contact request flow (approval) unless the target already added you
- **`private`**
  - Not discoverable for interaction
  - **Cannot be added as contact**
  - **Cannot receive messages**
  - Only the owner can view their own private profile

Practical enforcement:
- `CreateContact` forbids adding `private` profiles.
- `CheckContactExistance` (legacy symbol name in code) intentionally omits `recipient_user_id` when the target is private.

### 2.2 Username privacy (HMAC + encryption)
The personal profile system supports lookups without exposing plaintext usernames in the main `users` table:

- **HMAC index** (`users.hmac_sha256_hex_username`)
  - Used for searching/lookup (`GetUserByHashedUsername`)
  - Computed with `utils.ComputeHMAC(...)`

- **Encrypted username** (`users.b64_cipher_chacha20poly1305_username`)
  - Stored encrypted for at-rest privacy
  - Encrypted/decrypted via:
    - `utils.EncryptUsername(...)`
    - `utils.DecryptUsername(...)`

### 2.3 Admin-block behavior
The contact system and queries consistently avoid/admin-blocked users:

- On create-contact: the service checks if **you** are admin-blocked, and rejects.
- On create-contact: the service checks if the **target** is admin-blocked, and rejects.
- In many list queries: `cu.is_admin_blocked IS FALSE`.

---

## 3) Contact system (one-way contacts)

### 3.1 Data model
#### `user_contacts` (one-way)
Defined in `db/personal/migrations/002_personal_user_contacts.up.sql`.

- `owner_user_id` = the user who added someone
- `contact_user_id` = the user being added
- Composite PK: `(owner_user_id, contact_user_id)`

This means:
- If A adds B, it does **not** imply B added A.
- Mutual relationship exists only if both rows exist:
  - `(A -> B)` and `(B -> A)`

#### `contact_requests` (approval flow)
Defined in `db/personal/migrations/006_personal_contact_requests.up.sql`.

- Enum `request_status_enum`: `pending | accepted | declined`
- Unique pair: `(requester_user_id, receiver_user_id)`
- Trigger `auto_add_contact_on_accept`:
  - When request moves `pending -> accepted`, it inserts into `user_contacts` **only**:
    - `(requester_user_id -> receiver_user_id)`

So even after accepting a request:
- The requester gains a one-way contact.
- The receiver does **not** automatically add the requester.

### 3.2 API routes (personal domain)
Registered in `routes/personal_routes.go`:

- `GET  /personal/contacts/get`
- `POST /personal/contacts/check-existence`
- `POST /personal/contacts/create`
- `POST /personal/contacts/delete`
- `GET  /personal/contacts/requests/get`
- `POST /personal/contacts/requests/accept`
- `POST /personal/contacts/requests/reject`
- `POST /personal/contacts/requests/undo`
- `POST /personal/contacts/update-nickname`
- `POST /personal/contacts/remove-nickname`

All are behind `AuthSessionMiddleware`.

### 3.3 Contact flows (authoritative behavior)
All logic is implemented in:
- `personalservice/contact_service.go`

#### 3.3.1 Check existence (by username)
Endpoint:
- `POST /personal/contacts/check-existence`

Flow:
- Client sends `contact_username`.
- Backend computes `HMAC(contact_username)`.
- Backend fetches user via `GetUserByHashedUsername`.

Response behavior:
- If the target profile is `private`:
  - `exists=true`, `profile_type=private`, but **`recipient_user_id=null`**.
- If the target profile is `public` or `personal`:
  - returns `recipient_user_id`.

#### 3.3.2 Create contact
Endpoint:
- `POST /personal/contacts/create`

Hard rules:
- Cannot add yourself.
- Cannot add admin-blocked users.
- Cannot add private profiles.
- Must pass block checks (see Blocking section).

Behavior by `targetProfile.profile_type`:
- **public**
  - Insert `user_contacts(owner_user_id=self, contact_user_id=target)`.
- **personal**
  - If target already has you as contact (`target -> self` exists):
    - Insert `self -> target` directly.
  - Otherwise:
    - Create `contact_requests(self -> target)` (pending).

#### 3.3.3 Accept / reject / undo request
Endpoints:
- `POST /personal/contacts/requests/accept`
- `POST /personal/contacts/requests/reject`
- `POST /personal/contacts/requests/undo`

Important invariant:
- Accepting triggers DB to insert `requester -> receiver` contact (one-way).

#### 3.3.4 Listing contacts
Endpoint:
- `GET /personal/contacts/get`

Backend returns two lists:
- `contacts`: users **you added** (your `user_contacts` rows)
- `people_who_added_you`: users who added you (reverse lookup)

The service also computes `is_mutual` by cross-checking both lists.

---

## 4) Blocking system

### 4.1 Data model
Defined in `db/personal/migrations/004_personal_user_blocks.up.sql`:

- `user_blocks(blocker_user_id, blocked_user_id)` with unique pair

### 4.2 Automatic contact removal
Trigger `auto_remove_contact_on_block` runs after inserting a block:

- Deletes contact rows for both directions:
  - `(blocker -> blocked)`
  - `(blocked -> blocker)`

This ensures:
- Blocking is a hard privacy boundary.
- Contacts never remain connected after a block.

### 4.3 Contact creation blocking rule (implemented)
`CreateContact` calls sqlc query `IsEitherBlocked`:

- Returns:
  - `1` if requester blocked target
  - `2` if target blocked requester
  - `0` if no block

If blocked in either direction:
- Contact creation is rejected.

---

## 5) Profile privacy restrictions + exemptions (profile/avatar/status)

### 5.1 Data model
#### Global restrictions
`user_global_restrictions` (per-user global toggles)

- `restrict_profile`
- `restrict_avatar`
- `restrict_status`

#### Global exemptions
`user_global_restriction_exemptions` (per (owner, exempted_user) override)

- `exception_profile`
- `exception_avatar`
- `exception_status`

#### User-specific restrictions
`user_restrictions` (per (user, restricted_user) hide rules)

- `restrict_profile`
- `restrict_avatar`
- `restrict_status`

There are **no per-user exemptions** for `user_restrictions` (by design).

### 5.2 Circuit-breaker evaluation order (implemented for avatar visibility)
The contact queries return **raw flags**. The Go service applies priority logic.

Implemented in `personalservice/contact_service.go` via `shouldExposeAvatar(...)`:

Priority:
1. **Global profile restriction**
   - If `restrict_profile=true`:
     - show only if `exception_profile=true`
2. **Global avatar restriction**
   - If `restrict_avatar=true`:
     - show only if `exception_avatar=true`
3. **User-level profile restriction**
   - If `user_restrict_profile=true`:
     - hide
4. **User-level avatar restriction**
   - If `user_restrict_avatar=true`:
     - hide
Else show.

### 5.3 Scope note (important)
These restrictions currently govern **visibility** (avatar/profile/status).

Messaging permissions are **separate** (industry standard) and are handled by:
- Contacts (one-way)
- Blocking
- Profile type
- Primary device rules

If we later add a “who can message me” setting, it should be a messaging-specific field (not reusing avatar/profile flags).

---

## 6) Messaging eligibility rules (who can send to whom)

Before Phase 6 chat endpoints exist, the **business rules are already defined**:

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

---

## 7) Primary device (central session) rules

### 7.1 DB + invariant
`sessions.is_central` (auth DB) is enforced by a unique partial index:

- Exactly **one** `is_central=true` session per user.

### 7.2 Service endpoint
`POST /personal/settings/session/central`

Implementation:
- `personalservice/setting_service.go::SetCentralDevice`
  - Resets all sessions to non-central
  - Marks current session as central

### 7.3 Business rule constraints (product rules)
- Primary device must be **native** (iOS/Android), not web.
- Switching primary device should require old primary to be online.
  - If old primary is offline:
    - prompt that data will be lost
    - default behavior: do not allow switch without explicit confirmation

---

## 8) Phase 6 Chat system (primary-device-centric relay)

### 8.1 Core idea
The backend is a **temporary relay**, not a permanent message store.

- Messages are delivered to devices and then deleted from backend storage.
- Device encrypted SQLite is the long-term store.

### 8.2 Planned tables (Azure Postgres compatible)
Because Azure Postgres may not allow `pgcrypto`, IDs are generated in Go.

Planned tables:
- `chats`
- `chat_participants`
- `messages`

(Implementation will follow existing migration conventions: UUID PKs, timestamp trigger `set_timestamps()`, explicit indexes.)

### 8.3 Delivery rules (authoritative)
#### Send from primary device
- Backend stores message temporarily
- Deliver to recipient (any device) -> sets `delivered_to_recipient` (UI: Yellow Tick)
- Deliver to recipient **PRIMARY** -> sets `delivered_to_recipient_primary`
- After `delivered_to_recipient_primary = TRUE`: delete from backend

#### Send from secondary device (web/other)
- Backend stores message temporarily
- Deliver to recipient (any device) -> sets `delivered_to_recipient`
- Deliver to recipient **PRIMARY** -> sets `delivered_to_recipient_primary`
- Sync to sender **PRIMARY** -> sets `synced_to_sender_primary`
- After **BOTH** `delivered_to_recipient_primary = TRUE` AND `synced_to_sender_primary = TRUE`: delete from backend

### 8.4 Finalized edge-case policies (defaults)
These are the default policies to implement unless product requirements change:

- **Account creation**
  - Can happen from web or native
  - Sending or receiving messages is **not allowed** until user has an active primary device (native only)

- **Recipient offline**
  - Queue on backend until delivery
  - TTL default: **30 days** (aligned with common push TTL behavior)

- **Sender primary offline (secondary-device sends)**
  - Deliver to recipient first
  - Queue sender-primary sync
  - TTL default: **30 days**

- **Blocking**
  - Block immediately prevents:
    - new contacts
    - new messages
    - delivery of pending messages
  - Pending messages should be dropped/deleted

- **Account deletion**
  - Delete all interconnected data (FK cascade)

- **Delivery ACK failures / retries**
  - Use idempotency by `message_id`
  - Retry delivery with exponential backoff + jitter
  - Default: 5 attempts over ~minutes, then keep queued until TTL

- **Secondary-device rapid sends**
  - Rate limit secondary devices
  - Batch sync to primary when it comes online

- **Storage full on primary device**
  - Retry delivery
  - Notify user
  - TTL default: **7 days** for “storage full” delivery failures

- **Moderation**
  - Block + delete (ephemeral server means limited retroactive moderation)

- **Primary switching**
  - Require old primary online
  - If old primary offline: warn data loss; do not switch without explicit confirmation

### 8.7 Chat Status & Receipts (3-Icon System)

To support the ephemeral relay without storing permanent history, we use a **Metadata-based Read Status**:

#### 8.7.1 Visual States
- **Pending (Clock 🕒)**: Message stored on device, not yet acked by API.
- **Sent (Yellow Tick ✅)**: Message confirmed by server (`created_at` assigned). This also implicitly covers "Delivered" state internally.
- **Read (Green Tick ✅)**: Recipient has opened the chat.

#### 8.7.2 Logic Implementation
- **Sent/Delivered**: 
  - `delivered_to_recipient`: Tracks delivery to **ANY** recipient device. Controls UI status (Yellow Tick).
  - `delivered_to_recipient_primary`: Tracks delivery to **PRIMARY** recipient device. Controls data cleanup.
- **Read Status Calculation**:
  - **No per-message flag**: The `messages` table does *not* store a `read_at` timestamp.
  - **Metadata**: The `chats` table stores `p1_last_read_at` and `p2_last_read_at`.
  - **Calculated**: Frontend compares `message.created_at <= chat.other_user_last_read_at`.

#### 8.7.3 Synchronization Strategy (Catch-up Logic)
The system differentiates between individual and bulk acknowledgments:
1. **Outside Chat (Delivery ACK)**: When messages arrive while a user is on the Home screen/Inbox, the client automatically calls `/personal/chat/ack` (`acknowledged_by='recipient'`). This provides immediate "Delivered" feedback to the sender.
2. **Inside Chat (Read ACK)**: When a user opens a chat, the client calls `/personal/chat/mark-read`.
   - **Bulk Catch-up**: `MarkChatRead` internally performs a bulk delivery ACK for all messages in that chat. This ensures that even if individual delivery signals were missed (due to network drops), the state remains consistent.
   - **Primary Trigger**: On primary devices, `MarkChatRead` specifically marks messages as `delivered_to_recipient_primary = TRUE`, triggering the backend's ephemeral deletion logic.

#### 8.7.4 Deletion Rules (Ephemeral)
- Messages are eligible for deletion **ONLY** once:
  1. `delivered_to_recipient_primary = TRUE` (Recipient Primary confirmed read/open)
  2. **AND** `synced_to_sender_primary = TRUE` (for self-messages or secondary sends)
- Deletion is **independent** of visual read status, but requires **Primary Device** acknowledgement. A message delivered only to a secondary web client will **NOT** be deleted.

### 8.5 P2P WebRTC Sync Architecture (Secondary ↔ Primary)

#### 8.5.1 Design rationale
To reduce backend bandwidth and latency, secondary devices (web/other native) synchronize **directly with the primary device** using WebRTC peer-to-peer connections.

**Authorization flow:**
- Backend only handles WebRTC signaling (offer/answer/ICE) and authentication
- Actual sync data transfers via P2P data channel (bypasses backend)
- Backend validates session tokens before allowing P2P signaling

**Strict P2P requirement:**
- If P2P connection fails (NAT/firewall issues), there is **no fallback**.
- The user must be shown an error: "Connection cannot be established. Please try using the same network on both devices."

#### 8.5.2 WebRTC signaling server (backend)

Backend provides WebSocket endpoint for WebRTC signaling only:

```
POST /ws/signaling (authenticated WebSocket)

Message types:
- type: "webrtc-offer"      # Web → Backend → Primary
- type: "webrtc-answer"     # Primary → Backend → Web
- type: "ice-candidate"     # Either peer → Backend → Other peer
```

Backend responsibilities:
1. Authenticate both peers (verify JWT tokens)
2. Route signaling messages between primary and secondary
3. Enforce: only route to device with `is_central=true`
4. Track connection state per user (primary online/offline)

Backend does **not** see or store sync data (only signaling).

#### 8.5.3 Connection lifecycle

**Web client initiates:**
```
1. Web connects to WebSocket signaling server
2. Backend authenticates session
3. Backend checks if user has active primary device
4. If primary online:
   - Backend facilitates WebRTC handshake
   - Web creates offer → Backend relays to primary
   - Primary creates answer → Backend relays to web
   - ICE candidates exchanged via backend
   - Direct P2P data channel established
5. If primary offline:
   - Backend returns "primary-offline" message
   - Web UI shows "Primary device required" state
6. If P2P connection fails (e.g., ICE failure, strict NAT):
   - Connection aborts (no backend relay fallback).
   - Web UI shows error: "Connection cannot be established in this network. Try using the same network in both devices."
```

**Primary device responsibilities:**
```
1. Maintain persistent WebSocket connection to signaling server
2. Listen for WebRTC offers from secondary devices
3. Respond with WebRTC answers
4. Maintain P2P data channels with all connected secondaries
5. Serve sync requests directly via data channels
```

#### 8.5.4 Data channel protocol

Once P2P data channel is established:

**Sync request (Web → Primary via P2P):**
```json
{
  "type": "SYNC_REQUEST",
  "lastSyncTimestamp": 1234567890,
  "chatIds": ["chat_abc", "chat_def"]
}
```

**Sync response (Primary → Web via P2P):**
```json
{
  "type": "SYNC_RESPONSE",
  "messages": [
    {
      "id": "msg_123",
      "chatId": "chat_abc",
      "content": "...",
      "createdAt": 1234567900
    }
  ],
  "timestamp": 1234567950
}
```

**Media file request (Web → Primary via P2P):**
```json
{
  "type": "MEDIA_REQUEST",
  "messageId": "msg_123",
  "fileUri": "file:///path/to/media.jpg"
}
```

**Media file response (Primary → Web via P2P):**
```json
{
  "type": "MEDIA_RESPONSE",
  "messageId": "msg_123",
  "fileData": "base64_encoded_blob",
  "mimeType": "image/jpeg"
}
```

#### 8.5.5 NAT traversal (STUN/TURN configuration)

**STUN servers (for NAT discovery):**
- Primary: Google STUN servers (free)
  - `stun:stun.l.google.com:19302`
  - `stun:stun1.l.google.com:19302`

**TURN servers (for restrictive NATs/firewalls):**
- Consider: Cloudflare, Twilio, or self-hosted TURN
- Can help if direct P2P fails (corporate networks, symmetric NAT)
- Cost: ~$0.05/GB for relay bandwidth

**Configuration in code:**
```typescript
const peerConnection = new RTCPeerConnection({
  iceServers: [
    { urls: 'stun:stun.l.google.com:19302' },
    { urls: 'stun:stun1.l.google.com:19302' },
    // Add TURN if needed for enterprise environments
  ]
});
```

#### 8.5.6 Security model

**Transport encryption:**
- WebRTC data channels use DTLS (Datagram Transport Layer Security) automatically
- All P2P data is encrypted end-to-end between devices
- Backend cannot decrypt P2P sync data

**Authentication:**
- Backend validates JWT tokens before allowing signaling
- Primary device verifies session ownership before accepting P2P connections
- Secondary devices must prove ownership of same user account

#### 8.5.7 Storage on web (secondary devices)

Web clients use **IndexedDB as disposable cache only:**

**Rules:**
- Primary device SQLite = Source of truth (permanent)
- Web IndexedDB = Cache (temporary, 7-30 days TTL)
- Backend PostgreSQL = Ephemeral relay (deleted after delivery)

**Cache invalidation:**
- Primary device change → clear entire cache
- Message edit/delete from primary → invalidate specific items
- Cache age > 7 days → re-sync from primary
- User logout → clear cache

**Implementation:**
```typescript
// IndexedDB stores:
{
  metadata: {
    lastSyncTimestamp: number,
    primaryDeviceId: string
  },
  messages: Map<messageId, {data, cachedAt}>,
  media: Map<fileUri, {blob, cachedAt}>
}
```

#### 8.5.8 Connection Failure Handling

If P2P connection fails (NAT/firewall blocking):

**Detection:**
- WebRTC connection timeout (30 seconds)
- ICE connection state: "failed" or "disconnected"

**Failure flow:**
- Determine that the connection could not be established.
- Abort sync attempt.
- Display error to user: **"Connection cannot be established in this network. Try using the same network in both devices."**

#### 8.5.9 Implementation checklist

**Backend:**
- [ ] WebSocket signaling server (`/ws/signaling`)
- [ ] Session validation middleware
- [ ] Primary device tracking (per user)
- [ ] Signaling message routing logic

**React Native (Primary):**
- [ ] Install `react-native-webrtc`
- [ ] WebRTC peer connection setup (answerer role)
- [ ] Data channel message handlers
- [ ] SQLite query service for sync requests
- [ ] Media file serving from private storage

**Web (Secondary):**
- [ ] WebRTC peer connection setup (offerer role)
- [ ] Data channel message handlers
- [ ] IndexedDB cache management
- [ ] Cache invalidation logic
- [ ] UI: Show "same network" error on P2P connection failure

#### 8.5.10 Developer invariants

- P2P sync is **strictly required**, there is no backend relay fallback.
- Primary device SQLite is **always authoritative**
- Web cache can be cleared without data loss
- Backend must never store sync data permanently (other than ephemeral relays before primary ack)
- Signaling server must validate both peers before facilitating P2P

### 8.6 Media File Handling (ChatbasketDownloads + Web Browser API)

#### 8.6.1 Design philosophy

Media files follow a **two-step user-controlled workflow**:

1. **Step 1: Receive/download to private storage** (automatic)
   - Native: `expo-file-system` → `Paths.document` (private app folder)
   - Web: Synced via P2P → IndexedDB cache (temporary)

2. **Step 2: Save to public storage** (user-initiated)
   - Native Android: `ChatbasketDownloads` module → DCIM/Downloads (public)
   - Native iOS: Share sheet / Photos library
   - Web: Browser download → user's Downloads folder

**Why two steps:**
- Privacy: Media stays private until user explicitly saves
- Storage control: User decides what to keep publicly
- Permission-free: No storage permissions needed (Android 10+ Scoped Storage)

#### 8.6.2 ChatbasketDownloads Module (Android only)

**Purpose:** Save files from private app storage to public device storage (DCIM/Downloads) using MediaStore API.

**Module location:**
- `chatbasket/modules/chatbasket-downloads/`
- Documentation: `chatbasket/docs/CHATBASKET_DOWNLOADS_ANDROID.md`

**Platform support:**
- ✅ Android (MediaStore API)
- ❌ iOS (use native share sheet instead)
- ❌ Web (use browser download API)

**Key features:**
- Automatic directory routing by MIME type
- No permissions needed (Scoped Storage)
- Message ID tracking (for migration)
- Content URI generation
- Instant save (synchronous copy)
- Event-based completion notification

**Directory routing rules:**

| File Type | Directory | Subdirectory | Example |
|-----------|-----------|--------------|---------|
| Images | DCIM | `Chatbasket/images` | `/DCIM/Chatbasket/images/photo.jpg` |
| Videos | DCIM | `Chatbasket/videos` | `/DCIM/Chatbasket/videos/clip.mp4` |
| Audio | Download | `Chatbasket/audio` | `/Download/Chatbasket/audio/song.mp3` |
| PDFs | Download | `Chatbasket/documents` | `/Download/Chatbasket/documents/file.pdf` |
| Other | Download | `Chatbasket/files` | `/Download/Chatbasket/files/data.bin` |

**API usage:**

```typescript
import ChatbasketDownloads from '@/modules/chatbasket-downloads';
import { Paths, File } from 'expo-file-system';

// Step 1: Download to private storage
const fileName = 'photo_123.jpg';
const privateFile = new File(Paths.document, fileName);

const downloadedFile = await File.downloadFileAsync(
  'https://example.com/photo.jpg',
  privateFile
);

// Step 2: User taps "Save to Gallery"
if (Platform.OS === 'android') {
  const result = await ChatbasketDownloads.save(
    downloadedFile.uri,  // Private file URI
    fileName,
    messageId            // For migration tracking
  );
  
  // Returns:
  {
    success: true,
    folderName: "DCIM/Chatbasket/images",
    mimeType: "image/jpeg",
    directory: "DCIM",
    localUri: "content://media/external/images/media/1234",
    fileSize: 2048000,
    savedPath: "DCIM/Chatbasket/images/photo_123.jpg"
  }
}

// Listen for completion events
ChatbasketDownloads.addListener('onSaveComplete', (event) => {
  if (event.status === 'completed') {
    console.log('Saved:', event.fileName);
  }
});
```

**Integration with chat system:**
- Messages table stores `message_id`
- When user saves media, link `message_id → content_uri` in SQLite
- Used for device migration (query all saved files)

#### 8.6.3 Web Media File Handling

**Storage strategy:**

1. **Receive media via P2P:**
   ```typescript
   // Primary device sends media as base64 via data channel
   dataChannel.send(JSON.stringify({
     type: 'MEDIA_RESPONSE',
     messageId: 'msg_123',
     fileData: 'base64_encoded_data',
     mimeType: 'image/jpeg',
     fileName: 'photo_123.jpg'
   }));
   ```

2. **Cache in IndexedDB:**
   ```typescript
   // Convert base64 to Blob
   const blob = base64ToBlob(fileData, mimeType);
   
   // Store in IndexedDB
   const db = await openDB('chatbasket-cache', 1);
   await db.put('media', {
     blob,
     fileName,
     mimeType,
     cachedAt: Date.now()
   }, messageId);
   ```

3. **User saves file:**
   ```typescript
   // Browser download (standard approach)
   const handleSaveFile = async (messageId: string) => {
     // Get from cache
     const cached = await db.get('media', messageId);
     
     // Trigger browser download
     const url = URL.createObjectURL(cached.blob);
     const a = document.createElement('a');
     a.href = url;
     a.download = cached.fileName;
     a.click();
     URL.revokeObjectURL(url);
     
     // File saved to user's Downloads folder
   };
   ```

**Modern File System Access API (Chrome only):**

```typescript
// Optional: Let user choose save location
const saveWithPicker = async (blob: Blob, fileName: string) => {
  try {
    const handle = await window.showSaveFilePicker({
      suggestedName: fileName,
      types: [{
        description: 'Images',
        accept: {'image/*': ['.jpg', '.png']}
      }]
    });
    
    const writable = await handle.createWritable();
    await writable.write(blob);
    await writable.close();
  } catch (err) {
    // Fallback to standard download
    triggerDownload(blob, fileName);
  }
};
```

**Browser support:**
- ✅ Standard download: All modern browsers
- ⚠️ File System Access API: Chrome 86+, Edge 86+ only

#### 8.6.4 Platform comparison

| Feature | Android Native | iOS Native | Web |
|---------|---------------|------------|-----|
| **Private storage** | `Paths.document` | `Paths.document` | IndexedDB cache |
| **Public save API** | `ChatbasketDownloads` | Share sheet | Browser download |
| **Folder control** | ✅ Yes (DCIM/Downloads) | ❌ No (Photos library) | ⚠️ Limited (Chrome only) |
| **Default location** | `DCIM/Chatbasket/` | Photos app | `~/Downloads/` |
| **User can choose** | ❌ No | ❌ No | ✅ Yes (Chrome) |
| **Permissions needed** | ❌ None (API 29+) | ❌ None | ❌ None |
| **Migration tracking** | ✅ Yes (content URI) | ⚠️ Limited | ❌ No |

#### 8.6.5 Migration support

**Android (authoritative):**
```sql
-- SQLite schema on device
CREATE TABLE message_media (
  message_id TEXT PRIMARY KEY,
  content_uri TEXT NOT NULL,      -- content://... from MediaStore
  file_name TEXT NOT NULL,
  folder_name TEXT NOT NULL,      -- "DCIM/Chatbasket/images"
  mime_type TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  saved_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Migration flow:**
```
1. User switches primary device
2. Old primary: Query all saved files from message_media table
3. Old primary: Check file existence (getFileInfo)
4. Old primary: Transfer available files via WebSocket
5. New primary: Save files using ChatbasketDownloads
6. New primary: Recreate message_media table entries
```

**iOS/Web:**
- No persistence: Files saved to Photos/Downloads are not tracked
- Not suitable for migration (user manages files externally)

#### 8.6.6 Security considerations

**File access permissions:**
- Private storage: Only app can access (OS-enforced sandboxing)
- Public storage: All apps can access (user explicitly saved)
- Web cache: Only same-origin scripts can access (browser security)

**Content validation:**
- Validate MIME types before saving
- Scan for malicious content (optional, implement if needed)
- Limit file sizes (prevent DoS via storage exhaustion)

**Encryption:**
- Private storage: File content encrypted via ChaCha20-Poly1305 (optional)
- Public storage: Files saved unencrypted (user chose to make public)
- Web cache: Browser handles encryption (HTTPS + IndexedDB)

#### 8.6.7 Developer checklist

**Native (Android):**
- [ ] Install `chatbasket-downloads` module
- [ ] Install `expo-file-system` v17+
- [ ] Implement two-step workflow (download → save)
- [ ] Listen for `onSaveComplete` events
- [ ] Track saved files in SQLite (migration support)

**Native (iOS):**
- [ ] Install `expo-file-system` v17+
- [ ] Implement download to private storage
- [ ] Use iOS share sheet for public save
- [ ] No migration tracking (Photos library)

**Web:**
- [ ] Implement P2P media sync
- [ ] Cache media in IndexedDB
- [ ] Implement browser download trigger
- [ ] Optional: Add File System Access API (Chrome)
- [ ] Cache invalidation (clear on logout)

#### 8.6.8 Developer invariants

- Native must use two-step workflow (private → public)
- ChatbasketDownloads is Android-only (iOS uses share sheet, web uses browser API)
- Public saves are user-initiated only (never automatic)
- Migration tracking only supported on Android
- Web cache is disposable (not authoritative)

---

## 9) Developer checklist (do not break these invariants)

- Contacts are **one-way** by schema and triggers.
- Blocking must remain a hard boundary:
  - Contact creation must always check blocks
  - Block trigger must remove both-direction contacts
- Private profiles must remain non-contactable and non-messageable.
- Restrictions/exemptions must remain “visibility only”, messaging is separate.
- Primary device must remain unique (`sessions.is_central` invariant).
- Phase 6 chat must honor relay + deletion semantics (backend is not a message archive).

---

## 10) Message Delivery & Sync Architecture (Phase 6)

### 10.1 The "Single ACK" & Time-Based Delivery Flow

Chatbasket handles delivery acknowledgments (getting the "yellow double-tick" to the sender) using a heavily optimized, time-based algorithm to prevent network spam and database bloat.

#### The Problem
If User A sends 50 messages while User B is offline, sending an array of 50 `message_id` strings over the network when User B connects is inefficient.

#### The Solution: Time-Based Batching
1. **Recipient Trigger**: When User B opens the app or receives a batch of messages, their device finds the **latest** message and sends a single API call to `/personal/chat/ack` containing only that one `message_id`.
2. **Postgres Batch Update**: The backend database looks up the timestamp (`created_at`) of that target message. It runs a single SQL query to update `delivered_to_recipient = TRUE` for that message **and all older pending messages** in that chat. 
    *(See `MarkMessageDeliveredToRecipient` in `personal_chat.sql`)*
3. **Sender UI Optimization (`markMessagesDeliveredUpTo`)**: The backend broadcasts a `delivery_ack` WebSocket event back to User A containing **only** that target `message_id`. User A's frontend looks up the timestamp of that message in local state and instantly iterates backwards, marking all older local messages as "delivered" (yellow tick).

This entirely avoids array-mapping across the network while keeping the UI instantly synchronized.

### 10.2 Delivery Edge Cases Handled

The system is designed to trigger Delivery ACKs reliably across complex app states:

#### Fresh Start (Hydration Delay)
When a device wakes up from being completely closed, it calls `/personal/chat/pending`. The frontend fires the Delivery ACKs **immediately** upon receipt. It deliberately bypasses checking local `authState` or user ID hydration to ensure ACKs are not swallowed by the app's boot-up sequence.

#### Active Session (Home Screen Idle)
If a user is actively holding their phone but looking at the Home Screen (chat list), the WebSocket connection is alive. When a `new_message` event arrives, the background WebSocket bridge (e.g., `ws.event.bridge.ts`) intercepts it and fires the Delivery ACK to the server, even though the user hasn't tapped into the specific chat. This prevents messages from getting stuck on "Sent" (single gray tick) when the recipient physically has the data.

### 10.3 WebSocket Broadcasting vs. Primary Device Policy

It is critical to understand the distinction between the "Primary Device Policy" and WebSocket routing.

#### WebSockets are Agnostic (Optimistic UI)
WebSocket broadcasting intentionally **ignores** the Primary Device Policy. 
When the backend executes `BroadcastToUser` (for events like `new_message`, `delivery_ack`, `unsend`, or `delete_for_me`), the event is pushed concurrently to **ALL** active WebSocket connections for that user account, regardless of their `IsPrimary` flag.

**Why?**
WebSockets are used strictly for real-time visual UI updates. If a user has their phone (Primary) and laptop (Secondary) open simultaneously, both screens must instantly reflect the state change without lag.

#### Primary Policy maintains Authoritative Storage
The "Primary Device Policy" strictly dictates:
- **Authoritative Permanent Storage:** (Local SQLite).
- **Acknowledgment Loop Closures:** Only the Primary device is allowed to execute `MarkMessageSyncedToSenderPrimary`, which tells the server it is safe to permanently delete the ephemeral message payload.

#### The Sync Actions Fallback
Because WebSockets are volatile (devices can go offline), the system utilizes a `message_sync_actions` table in Postgres for state changes like *unsends* and *read receipts*.
- **Online:** The device receives the real-time WebSocket event (`WSEventUnsend`) and updates the UI instantly.
- **Offline:** The device misses the WebSocket event. When the frontend boots up, its internal `$syncEngine` runs a `catchUp()` routine that queries the `/personal/chat/sync-actions` endpoint to fetch, apply, and acknowledge any revocations (`unsend`, `delete_for_me`) it missed while disconnected.
