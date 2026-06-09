# E2EE Upgrade — ChatBasket

End-to-End Encryption design for ChatBasket personal messaging.

---

## 1. Library

**`react-native-libsodium`** (already installed).

### Alternatives Rejected

| Alternative | Why Rejected |
|---|---|
| `react-native-quick-crypto` | Node.js `crypto` API — requires manual ECDH + HKDF + cipher chaining (5+ steps, high misuse risk). No web parity — would need separate WebCrypto codepath. New native dependency for zero benefit. |
| Nostr NIP-44 | Uses secp256k1 (Bitcoin curve) — 40% slower than X25519, only exists for Bitcoin key compatibility. Encrypt-then-MAC composition vs our built-in AEAD. Padding/versioning unnecessary for centralized app. |
| `crypto_aead_xchacha20poly1305_ietf` | Same speed as secretbox, but native JSI binding is buggy — throws on non-string `additional_data`. AAD unnecessary for our use case. |
| `crypto_box_seal` | Anonymous sender — no authentication. 48 bytes overhead vs 16. |
| `crypto_box_beforenm` / `crypto_kx` | Missing from native JSI bindings. Web-only. |

---

## 2. Algorithms

### Text Messages — `crypto_box_easy`

Single function call. Internally does:

1. **X25519** (Curve25519) — ECDH key agreement (~0.3ms on mobile)
2. **XSalsa20** (256-bit) — stream cipher (~3 GB/s)
3. **Poly1305** — MAC for integrity + sender authentication

```
ciphertext = crypto_box_easy(message, nonce, recipientPublicKey, senderPrivateKey)
plaintext  = crypto_box_open_easy(ciphertext, nonce, senderPublicKey, recipientPrivateKey)
```

### Media Files — Envelope Encryption

1. `crypto_secretbox_keygen()` → random 256-bit symmetric key
2. `crypto_secretbox_easy(mediaBytes, nonce, symmetricKey)` → encrypt blob at full speed
3. `crypto_box_easy(symmetricKey, nonce2, recipientPub, senderPriv)` → encrypt the 32-byte key

Why: Separates bulk encryption from identity. Enables encrypting the blob once and wrapping the key per-recipient (future group support).

**Local Storage & Upload Rules:**
- **Appwrite File Names:** Encrypted files uploaded to Appwrite are renamed to append `.enc` (e.g., `image.png` is uploaded as `image.png.enc`). This indicates to developers and the Appwrite admin panel that the file is encrypted, and prevents Appwrite from trying to generate previews (which would fail).
- **Original Metadata:** The original file name and MIME type (e.g., `image/png`) are stored in the message metadata in the database so the recipient can reconstruct the original file type upon decryption.
- **App-Private Storage:** Downloaded media files are decrypted and stored in the app's private sandbox directory (e.g., `FileSystem.documentDirectory` on mobile) in decrypted form to prevent decryption lag during view/scroll.
- **Public Gallery:** The decrypted file is only saved to the user's public Photos Gallery if they explicitly tap "Save to Gallery".

---

## 3. Key Lifecycle

**Strict E2EE — Accept Data Loss.**

- Private keys generated once per device, stored in **Expo Secure Store** (mobile) / **sessionStorage** (web).
- **Never uploaded** to server. No recovery phrase. No backup.
- **Lost phone / deleted app** → private key gone, undelivered relay messages unrecoverable.
- **Manual logout** → app fetches and decrypts all pending messages before deleting local key.
- **Key reset** → server purges Bob's undelivered relay messages (undecryptable with new key).
- **Preview decryption failure** → chat list previews are best-effort only. If the last-message preview cannot be decrypted because either sender or recipient key changed/reset, the client must set the preview text to an empty string (`""`) instead of showing ciphertext or an error.

---

## 4. Implementation Phases

### Phase 1: E2EE (Mobile-Only)

Native-to-native encrypted messaging. Web continues working as-is (unencrypted).

**Steps:**

1. **Key Generation** — On first launch (or account creation), call `crypto_box_keypair()`. Store private key in Expo Secure Store. Upload public key to server.
2. **Server Changes** — Store public key per user. When sending a message, client encrypts before sending. Server relays ciphertext blindly.
3. **Encrypt on Send** — Sender fetches recipient's public key from server, calls `crypto_box_easy(message, nonce, recipientPub, senderPriv)`.
4. **Decrypt on Receive** — Recipient calls `crypto_box_open_easy(ciphertext, nonce, senderPub, recipientPriv)`.
5. **Graceful Degradation** — Server checks if recipient has a public key. If yes → client encrypts. If no (web-only user without key sync) → plaintext as today. No breaking change.

**Offline Sending Rule — Never encrypt offline, always encrypt at send time.**

- Messages are always stored as **plaintext in the local queue** (on device), regardless of connectivity.
- Encryption happens **only at send time** when the device is online.
- Send process: fetch/verify recipient's **latest** `e2ee_public_key` (using the cached key validation flow below) → encrypt → send ciphertext → clear local queue.
- If recipient's key is `NULL` → send plaintext (graceful degradation).

**Key Cache & Validation Flow**

To avoid querying the server before sending every single message, the client caches recipient public keys locally. To verify if a cached key is up-to-date:

1. **Metadata Sync (Active validation)**:
   - When retrieving the chat list (e.g. `GET /api/chats`) or room state, the server payload includes the other user's latest `e2ee_public_key` or its hash.
   - The client compares this payload with its local cache and updates the cache if a mismatch is detected.
2. **[FUTURE] Decryption Failure Recovery**:
   - If Alice sends a message encrypted with a stale key, Bob's device detects decryption failure (`crypto_box_open_easy` fails).
   - In a future update, Bob's client will automatically reply with a hidden system notification to Alice's client, prompting it to invalidate the cache, fetch Bob's new key, and automatically re-encrypt/re-send the message.

### Phase 2: WebRTC Key Sync (Web)

Web login stays email/password (unchanged). Private key is **automatically** fetched from native device — no QR, no pairing code, no user action.

**Flow:**

```
┌──────────────┐         ┌──────────┐         ┌──────────────┐
│   Web Client │         │  Server  │         │  Native App  │
│ (logged in   │         │          │         │ (has private │
│  via email)  │         │          │         │   key)       │
└──────┬───────┘         └────┬─────┘         └──────┬───────┘
       │                      │                      │
       │  1. Web detects no   │                      │
       │     local private    │                      │
       │     key in session   │                      │
       │                      │                      │
       │  2. Web sends        │                      │
       │     "key_sync_request"│                      │
       │─────────────────────►│                      │
       │                      │                      │
       │                      │  3. Server pushes    │
       │                      │     "key_sync_request"│
       │                      │     via WebSocket/    │
       │                      │     push notification │
       │                      │─────────────────────►│
       │                      │                      │
       │                      │  4. Native auto-     │
       │                      │     creates WebRTC   │
       │                      │     offer             │
       │                      │◄─────────────────────│
       │                      │                      │
       │  5. WebRTC signaling │                      │
       │     (SDP exchange    │                      │
       │      via server)     │                      │
       │◄────────────────────►│◄────────────────────►│
       │                      │                      │
       │  6. P2P data channel established            │
       │     (DTLS encrypted)                        │
       │◄═══════════════════════════════════════════►│
       │                      │                      │
       │  7. Native sends private key (32 bytes)     │
       │     over encrypted data channel             │
       │◄══════════════════════════════════════════──│
       │                      │                      │
       │  8. Web stores key   │                      │
       │     in sessionStorage│                      │
       │                      │                      │
       │  9. Web can now      │                      │
       │     decrypt messages │                      │
       └──────────────────────┘                      │
```

**Key details:**

- **Fully automatic** — no QR code, no pairing code. Web opens → detects missing key → server notifies native → WebRTC → key arrives. Zero user interaction.
- **Auth is the session** — both web and native are logged into the same account (same user ID). Server only relays `key_sync_request` to devices belonging to the same user.
- **Server is signaling only** — server handles **only** WebRTC connection management (SDP offer/answer, ICE candidates). The private key **never** passes through the server. All data flows exclusively through the WebRTC P2P data channel (DTLS encrypted end-to-end between web and native).
- **Native must be online** — if native app is not open/reachable, web shows "Open your phone app to sync encryption keys." Retries automatically when native comes online.
- **WebRTC data channel** — DTLS-encrypted P2P. The 32-byte private key is the only payload.
- **Session-scoped** — web stores private key in `sessionStorage`. Key is lost when tab closes. Auto-syncs again on next visit.
- **Single public key on server** — server stores only the native device's public key. Both native and web decrypt with the same private key.

---

## 5. Cross-Platform Availability

All functions verified in `node_modules/react-native-libsodium/src/lib.native.ts` (JSI) and `lib.ts` (WASM):

| Function | Native (JSI) | Web (WASM) |
|---|---|---|
| `crypto_box_keypair` | ✅ | ✅ |
| `crypto_box_easy` | ✅ | ✅ |
| `crypto_box_open_easy` | ✅ | ✅ |
| `crypto_secretbox_keygen` | ✅ | ✅ |
| `crypto_secretbox_easy` | ✅ | ✅ |
| `crypto_secretbox_open_easy` | ✅ | ✅ |
| `randombytes_buf` | ✅ | ✅ |
| `crypto_sign_keypair` | ✅ | ✅ |
| `crypto_sign_detached` | ✅ | ✅ |
| `crypto_sign_verify_detached` | ✅ | ✅ |

---

## 6. Database Schema Changes

**Migration**: `008_e2ee_public_key.up.sql`

### `users` table — ADD `e2ee_public_key`

```sql
ALTER TABLE users ADD COLUMN e2ee_public_key TEXT CHECK (
    e2ee_public_key IS NULL OR length(e2ee_public_key) = 44
);
```

- Base64-encoded 32-byte X25519 public key (44 chars).
- `NULL` = user hasn't set up E2EE yet → server sends plaintext (graceful degradation).
- Placed on `users` (personal DB), not `auth_users` (common DB) — the public key is a messaging identity concern, not auth. Same DB as `chats` and `messages`, avoids cross-DB joins.

### `messages` table — REMOVE CHECK constraint on `content`

- **Change:** Removed `CHECK (length(content) <= 5000)` constraint from `content` column.
- **Why:** Encryption and Base64 encoding introduce ~33% overhead. A 5,000-character message becomes ~6,720 characters after encryption + nonce.
- **Validation:** The 5,000-character message limit is now enforced in the Application code (frontend/backend APIs) rather than the database level.

### No other schema changes

- **Nonce**: Prepended to ciphertext in existing `content` field (industry standard — Signal, WhatsApp do the same). Format: `base64(nonce_24_bytes || ciphertext)`.
- **Wrapped key (media)**: Stored in `content` field for media messages. Format: `base64(nonce || encrypted_symmetric_key)`. Encrypted file blob stored via existing `file_id`.
- **Private key**: Never stored in DB. Device-only (Expo Secure Store / sessionStorage).
