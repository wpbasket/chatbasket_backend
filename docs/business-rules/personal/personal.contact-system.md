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