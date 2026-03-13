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

## 8) Chat system (primary-device-centric relay)

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