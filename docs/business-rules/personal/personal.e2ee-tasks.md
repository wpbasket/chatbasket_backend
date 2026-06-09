# E2EE Upgrade Checklist

Checklist tracking the design, database, backend, and frontend tasks for the ChatBasket End-to-End Encryption (E2EE) upgrade.

## 1. Design & Specification
- [x] Select crypto library (`react-native-libsodium`)
- [x] Formulate encryption & decryption algorithms (`crypto_box_easy` for text, envelope encryption for media)
- [x] Define offline message queueing & encryption rules (always encrypt at send time, store plaintext locally)
- [x] Define cached key validation strategy (metadata sync, WS push, and future decryption failure recovery)
- [x] Define media upload and local storage rules (encrypted upload to Appwrite, decrypted storage in private sandbox)

## 2. Database Migrations
- [x] Audit all up/down migrations for re-run safety (idempotency)
- [x] Create migration to add `e2ee_public_key` column to `users` table (Migration 008)
- [x] Create/update migration to remove `CHECK (length(content) <= 5000)` constraint from `messages` table (Migration 006)
- [x] Apply migrations to development database

## 3. Backend Implementation (Next)
- [x] Add endpoint to upload/save user's `e2ee_public_key`
- [x] Add endpoint to fetch another user's `e2ee_public_key` by user ID
- [x] Update chat list / metadata API to include participant public keys for local caching
- [x] Update message delivery APIs to return sender public key (enables decryption)
  - [x] Text send: REST `POST /personal/chat/send`, WS `send_message`, and WS `new_message` payloads include `sender_e2ee_public_key`
  - [x] Message retrieval: `GET /personal/chat/messages` includes `sender_e2ee_public_key` for each message
  - [x] Pending relay/sync: `GET /personal/chat/pending` includes `sender_e2ee_public_key` for each message
  - [x] File messaging: REST `POST /personal/chat/upload` response and WS `new_message` file payload include `sender_e2ee_public_key`
  - [x] Chat list preview rule documented: if last-message preview decryption fails after key reset/change, client displays empty string (`""`)

## 4. Frontend Implementation (Phase 1: Native E2EE)
- [ ] Generate X25519 identity keypair on first launch
- [ ] Store private key securely using `Expo Secure Store`
- [ ] Upload public key to the backend database
- [ ] Implement client-side key caching and background metadata sync
- [ ] Integrate text encryption (`crypto_box_easy`) on message send
- [ ] Integrate text decryption (`crypto_box_open_easy`) on message receive
- [ ] Implement media envelope encryption (symmetric key generation, encrypting file, uploading to Appwrite, wrapping key)
- [ ] Implement media decryption (downloading file, decrypting key, decrypting file, saving to private sandbox)

## 5. Phase 2: WebRTC Key Sync (Web)
- [ ] Implement web key check on launch
- [ ] Implement `key_sync_request` WebSocket signal relay on server
- [ ] Implement WebRTC SDP/ICE candidate exchange signaling on server
- [ ] Implement WebRTC P2P DTLS data channel setup on native app
- [ ] Implement automatic key transfer over P2P data channel
- [ ] Store key in web `sessionStorage` and enable web decryption
