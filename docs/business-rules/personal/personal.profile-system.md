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