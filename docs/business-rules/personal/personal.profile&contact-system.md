# Personal Profile System - Business Rules

## 1. Profile Types

### 1.1 Overview
The system supports three profile types that determine how users can be discovered and contacted.

### 1.2 Profile Type Definitions

#### 1.2.1 Public Profile
- The user is discoverable by other users
- Other users may add this user as a contact directly without approval
- No contact request flow is required

#### 1.2.2 Personal Profile
- The user is discoverable by other users
- Adding this user as a contact requires approval via contact request flow
- Exception: If the target user has already added the requester as a contact, the requester may add them directly without approval

#### 1.2.3 Private Profile
- The user is not discoverable for interaction purposes
- Other users cannot add this user as a contact
- This user cannot receive messages from other users
- Only the profile owner may view their own private profile

### 1.3 Enforcement Rules
- The system shall reject any attempt to create a contact relationship with a private profile
- When checking if a user exists, the system shall confirm existence for private profiles but shall not reveal the user identifier

---

## 2. Username Privacy

### 2.1 Purpose
The system implements multiple storage mechanisms for usernames to balance security, performance, and functionality requirements.

### 2.2 Username Storage Methods
The system maintains three forms of username storage:

#### 2.2.1 Plaintext Username Storage
- The system shall store plaintext usernames in a separate table with random identifiers
- Purpose: Enable username availability checks and system operations requiring plaintext access
- Storage location: Dedicated table with no direct user relationship
- The plaintext username table uses random UUID identifiers unlinked to user accounts
- **Use cases:**
  - Username availability validation during registration
  - Username uniqueness enforcement
  - System-level username operations that require plaintext access

#### 2.2.2 Username Hashing for Lookup
- The system shall maintain a cryptographic hash of each username in the user profile table
- Purpose: Enable username lookup without exposing plaintext values in the main user table
- The system shall use HMAC-SHA256 for computing username hashes
- All username searches for user identification shall be performed against the hashed value
- **Use cases:**
  - User lookup by username (e.g., "Check if contact exists" feature)
  - User authentication and identification
  - Contact search operations
  - Prevents exposure of plaintext usernames in the main user table

#### 2.2.3 Username Encryption at Rest
- The system shall store usernames in encrypted form in the user profile table
- Purpose: Protect usernames from unauthorized access at rest in the main user table
- The system shall use ChaCha20-Poly1305 authenticated encryption
- The system shall decrypt usernames only when needed for authorized operations
- **Use cases:**
  - Displaying username to the authenticated user
  - Returning username in API responses to authorized users
  - Profile display operations
  - Provides additional security layer for username storage in the main user table

### 2.3 Username Generation and Format
- Username format: 4 uppercase letters (A-Z) followed by 6 digits (0-9), total 10 characters
- Example: ABCD123456
- Generation uses cryptographically secure random number generation
- The system shall enforce exactly 10 characters for all usernames

### 2.4 Username Processing Order
During profile creation, the system shall process usernames in the following order:
1. Generate random username using secure random generation
2. Compute HMAC-SHA256 hash of the username
3. Encrypt the username using ChaCha20-Poly1305
4. Store hashed and encrypted forms in the user profile table
5. Store plaintext username in the separate plaintext username table

### 2.5 Encryption and Hashing Implementation Details

#### 2.5.1 HMAC-SHA256 Hashing
- Algorithm: HMAC with SHA-256
- Key: Single secret key used for all usernames
- Output: 64-character hexadecimal string
- The same secret key is required for both hashing and verification

#### 2.5.2 ChaCha20-Poly1305 Encryption
- Algorithm: ChaCha20-Poly1305 authenticated encryption (AEAD)
- Key: Same secret key used for HMAC (32 bytes)
- Nonce: First 12 bytes of the user's UUID (deterministic, user-specific)
- Output: Base64-encoded ciphertext with prepended nonce (max 52 characters)
- Nonce is stored with the ciphertext for decryption

### 2.6 Security Rationale
The three-form storage approach provides defense in depth:
- **Plaintext table**: Isolated with random IDs, enables functionality without linking to user accounts
- **Hashed form**: Enables lookups without exposing plaintext in the main user table
- **Encrypted form**: Protects usernames at rest while allowing authorized decryption when needed

### 2.7 Decryption and Irreversibility

#### 2.7.1 HMAC-SHA256 Hash (Irreversible)
- The hashed form is **cryptographically irreversible**
- Cannot be decrypted or converted back to plaintext
- One-way transformation: plaintext + secret key → hash (no reverse operation exists)
- Verification method: Compare hash of input username (computed with secret key) with stored hash
- The secret key is required for both hashing and verification
- Security benefit: Even with database access, attackers cannot recover plaintext usernames from hashes without the secret key

#### 2.7.2 ChaCha20-Poly1305 Encryption (Reversible with Key)
- The encrypted form is **reversible with the correct encryption key**
- Decryption process: encrypted username + encryption key → plaintext username
- The system decrypts usernames when:
  - Authenticated user requests their own profile
  - System needs to display username in authorized API responses
  - Contact list operations require username display
  - Contact request lists require username display
- Decryption is performed server-side only
- The encryption key is stored securely and never exposed to clients
- Security benefit: Protects usernames at rest; requires both database access AND encryption key to recover plaintext

### 2.8 Username Validation Rules
- Frontend validation: Must be exactly 10 characters after whitespace removal
- Frontend input: Split into two fields (4 letters + 6 numbers) with auto-uppercase conversion
- Backend validation: Enforced via database constraints (exactly 10 characters)
- Username lookup: Case-insensitive (converted to uppercase before hashing)

---

## 3. Administrative Block Enforcement

### 3.1 Contact Creation Rules
- The system shall reject contact creation if the requester has been administratively blocked
- The system shall reject contact creation if the target user has been administratively blocked

### 3.2 Query Filtering
- All contact list queries shall exclude administratively blocked users from results

---

## 4. Contact System (One-Way Relationships)

### 4.1 Data Model

#### 4.1.1 Contact Relationships
- Contact relationships are unidirectional (one-way)
- Each contact relationship consists of:
  - Contact owner: The user who added the contact
  - Contact target: The user being added as a contact
- A unique constraint exists on the combination of contact owner and contact target

#### 4.1.2 Contact Relationship Rules
- If User A adds User B, this does not imply User B has added User A
- A mutual contact relationship exists only when both directional relationships exist:
  - Relationship 1: User A has added User B
  - Relationship 2: User B has added User A

#### 4.1.3 Contact Requests
- Contact requests support three status values: pending, accepted, declined
- A unique constraint exists on the combination of requester and receiver
- The system maintains an automated process for contact creation upon request acceptance

#### 4.1.4 Contact Request Acceptance Behavior
- When a contact request transitions from pending to accepted, the system shall automatically create a one-way contact relationship from requester to receiver
- The receiver does not automatically gain a contact relationship to the requester
- To establish a mutual relationship, the receiver must separately add the requester

### 4.2 Contact Operations

#### 4.2.1 Check Contact Existence
- Input: Username of the user to check
- Process:
  1. Frontend converts username to uppercase
  2. System computes HMAC-SHA256 hash of the provided username
  3. System queries user database using the hashed value
  4. System validates user is not checking themselves
- Response rules:
  - For private profiles: Confirm existence and profile type, but do not reveal user identifier
  - For public or personal profiles: Return user identifier and profile type
- Error conditions:
  - User not found: Return appropriate error message
  - Self-check: Return exists as false

#### 4.2.2 Create Contact
- Validation rules (system shall reject if violated):
  1. Payload must not be null and contact user ID must not be empty
  2. Contact user ID must be valid UUID format
  3. User cannot add themselves as a contact (returns "self_addition" error)
  4. Requester must not be administratively blocked (returns "self_admin_blocked" error)
  5. Target user must exist (returns "user_not_found" error)
  6. Target user must not be administratively blocked (returns "user_admin_blocked" error)
  7. Block status check using bidirectional query:
     - If requester blocked target: Return "you_blocked_user" error
     - If target blocked requester: Return "user_blocked_you" error
  8. Contact must not already exist (returns "already_in_contacts" success message)
  9. User cannot add users with private profiles (returns "user_private_profile" error)
  10. Nickname validation: If provided, must be 40 characters or less (Unicode character count)

- Behavior by target profile type:
  - **Public profile**:
    - System shall create contact relationship immediately
    - Returns "public_contact_added" success message
  - **Personal profile**:
    - If target has already added requester: System shall create contact relationship immediately, returns "personal_contact_added"
    - Otherwise: System shall create or update contact request:
      - If pending request exists: Return "pending_request_exists" without creating duplicate
      - If accepted or declined request exists: Delete old request and create new one
      - Returns "contact_request_sent" success message

- Contact creation uses ON CONFLICT DO NOTHING for idempotency

#### 4.2.3 Contact Request Management

##### 4.2.3.1 Accept Request
- Validation sequence:
  1. Payload must not be null and contact user ID must not be empty
  2. Contact user ID must be valid UUID format
  3. User cannot accept their own request (returns "self_action_not_allowed" error)
- Database operation:
  - Updates request status from "pending" to "accepted"
  - Returns outcome: "accepted", "not_found", or "processed"
- Automatic contact creation:
  - Database trigger fires AFTER UPDATE when status changes from "pending" to "accepted"
  - Creates one-way contact relationship: requester → receiver
  - Copies nickname from contact request to contact record
  - Uses ON CONFLICT DO NOTHING to prevent duplicates
- Response messages:
  - Success: "contact_request_accepted"
  - Not found: "pending_request_not_found" (404)
  - Already processed: "request_already_processed" (409)
- Frontend behavior:
  - Checks if requester is already in accepter's contacts
  - If mutual: Updates UI to show mutual badge
  - Adds requester to "People who added you" list
  - Shows different success messages for mutual vs non-mutual contacts

##### 4.2.3.2 Reject Request
- Validation sequence:
  1. Payload must not be null and contact user ID must not be empty
  2. Contact user ID must be valid UUID format
  3. User cannot reject their own request (returns "self_action_not_allowed" error)
- Database operation:
  - Updates request status from "pending" to "declined"
  - Returns same outcome types as accept: "accepted", "not_found", or "processed"
- Response messages:
  - Success: "contact_request_declined"
  - Not found: "pending_request_not_found" (404)
  - Already processed: "request_already_processed" (409)
- Frontend behavior:
  - Shows confirmation dialog before rejecting
  - Removes request from pending list
  - Removes user from "People who added you" list if present

##### 4.2.3.3 Undo Request
- Validation sequence:
  1. Payload must not be null and contact user ID must not be empty
  2. Contact user ID must be valid UUID format
  3. User cannot undo another user's request (returns "self_action_not_allowed" error)
- Database operation:
  - Deletes the pending request record entirely
- Response messages:
  - Success: "contact_request_undone"
  - Not found: "pending_request_not_found" (404)
- Frontend behavior:
  - Shows confirmation dialog before undoing
  - Removes request from sent requests list

#### 4.2.4 List Contacts
- Response structure:
  - Outgoing contacts: List of users the requester has added
  - Incoming contacts: List of users who have added the requester
  - Mutual indicator: Computed flag indicating bidirectional contact relationship
- Mutual detection logic:
  - Backend creates map of users who added the requester
  - For each outgoing contact, checks if they exist in the map
  - Sets mutual flag to true if bidirectional relationship exists
- Query filtering:
  - Excludes private profiles
  - Excludes administratively blocked users
  - Applies privacy restrictions to avatar visibility
- Username decryption:
  - System decrypts encrypted usernames for display
  - Returns decrypted username in API response

#### 4.2.5 Delete Contact
- Validation rules:
  1. Payload must not be null and contact user ID array must not be empty
  2. Each contact user ID must be non-empty after trimming
  3. Each contact user ID must be valid UUID format
  4. User cannot delete themselves as contact (returns "self_action_not_allowed" error)
- Batch deletion support:
  - Accepts array of contact user IDs
  - Deduplicates IDs using map to prevent duplicate operations
  - Fails fast on first invalid ID (no partial deletion)
- Deletion behavior:
  - One-way deletion only (removes only requester's side of relationship)
  - Does NOT automatically remove reverse relationship
  - Other user maintains their contact entry unchanged
- Response messages:
  - Single deletion: "contact_deleted"
  - Multiple deletions: "contacts_deleted"
  - Partial success: "contacts_deleted_partial" (some contacts not found)
  - Not found: "contact_not_found" (404)
- Frontend behavior:
  - Shows confirmation dialog before deletion
  - Removes contact from contacts list
  - Updates mutual status to false in "People who added you" if applicable
  - Contact remains visible in "People who added you" if they still have requester added

#### 4.2.6 Nickname Operations

##### 4.2.6.1 Update Nickname
- Validation rules:
  1. Payload must not be null and contact user ID must not be empty
  2. Contact user ID must be valid UUID format
  3. User cannot update nickname for themselves (returns "self_action_not_allowed" error)
  4. Contact must exist (returns "contact_not_found" error)
  5. Nickname validation:
     - Trims whitespace before validation
     - If trimmed string is empty: Sets nickname to null (removes nickname)
     - If trimmed string exceeds 40 characters (Unicode character count): Returns "invalid_nickname_length" error
- Database operation:
  - Updates nickname field in user_contacts table
  - Stores trimmed value or null
- Response message: "contact_nickname_updated"
- Frontend behavior:
  - Input field enforces 40 character limit in real-time
  - Slices input at character boundary to prevent exceeding limit
  - Shows modal dialog for nickname editing
  - Empty trimmed strings converted to null before submission

##### 4.2.6.2 Remove Nickname
- Implementation: Calls update nickname operation with nickname set to null
- Validation: Same as update nickname
- Response message: "contact_nickname_removed"
- Frontend behavior:
  - Shows confirmation before removal
  - Updates local state to set nickname to null

---

## 5. User Blocking

### 5.1 Block User Operation
- Validation sequence:
  1. Payload must not be null and blocked user ID must not be empty
  2. Blocked user ID must be valid UUID format
  3. User cannot block themselves (returns "self_block_not_allowed" error, 409 Conflict)
  4. Target user must exist (returns "user_not_found" error, 404)
  5. Target user must not be administratively blocked (returns "user_admin_blocked" error, 403)
  6. Existing block check using bidirectional query:
     - If requester already blocked target: Return success (idempotent operation)
     - If target already blocked requester: Allow reciprocal block to proceed

### 5.2 Automatic Mutual Contact Removal
- Database trigger fires AFTER INSERT on user_blocks table
- Automatically removes both directional contact relationships:
  - Blocker → Blocked contact removed
  - Blocked → Blocker contact removed
- Uses tuple deletion pattern for performance
- Trigger function: remove_contact_on_block()

### 5.3 Bidirectional Block Checking
- Query: IsEitherBlocked returns status code:
  - 0: No block exists between users
  - 1: Requester blocked target ("you_blocked_user")
  - 2: Target blocked requester ("user_blocked_you")
- Used in multiple operations:
  - Contact creation: Prevents adding blocked users
  - Message sending: Prevents messaging blocked users
  - Block creation: Checks for existing blocks

### 5.4 Message Cleanup on Block
- System automatically deletes pending messages between users in both directions
- Ensures no pending messages remain after block is created

### 5.5 Block Cascade Behavior
- If user account is deleted: All block records are automatically removed via ON DELETE CASCADE
- Blocks are permanent until explicitly removed or account deleted

### 5.6 Frontend Implementation Status
- Backend blocking functionality is fully implemented
- Frontend has no blocking UI or API integration
- No block action available in contact menus
- No confirmation dialogs for blocking

---

## 6. Profile Privacy Restrictions

### 6.1 Restriction Types

#### 6.1.1 Global Restrictions
- Scope: Applies to all users except those with explicit exemptions
- Restriction categories:
  - Profile information restriction: Hide profile information globally
  - Avatar restriction: Hide avatar globally
  - Status restriction: Hide status globally

#### 6.1.2 Global Exemptions
- Scope: Per-user override of global restrictions
- Exemption categories:
  - Profile exemption: Allow specific user to view profile despite global restriction
  - Avatar exemption: Allow specific user to view avatar despite global restriction
  - Status exemption: Allow specific user to view status despite global restriction

#### 6.1.3 User-Specific Restrictions
- Scope: Hide information from specific users
- Restriction categories:
  - Profile restriction: Hide profile from specific user
  - Avatar restriction: Hide avatar from specific user
  - Status restriction: Hide status from specific user
- Note: User-specific restrictions do not support exemptions

### 6.2 Visibility Evaluation Rules

The system shall evaluate visibility in the following priority order (circuit breaker pattern):

1. **Global profile restriction check (Priority 1)**
   - If global profile restriction is enabled: Show only if profile exemption exists for the requesting user
   - If restriction applies and no exemption exists: Hide and skip further checks
   - Short-circuits evaluation if triggered

2. **Global avatar restriction check (Priority 2)**
   - If global avatar restriction is enabled: Show only if avatar exemption exists for the requesting user
   - If restriction applies and no exemption exists: Hide and skip further checks
   - Short-circuits evaluation if triggered

3. **User-specific profile restriction check (Priority 3)**
   - If profile restriction exists for the requesting user: Hide and skip further checks
   - No exemption mechanism available
   - Short-circuits evaluation if triggered

4. **User-specific avatar restriction check (Priority 4)**
   - If avatar restriction exists for the requesting user: Hide
   - No exemption mechanism available

5. **Default behavior**
   - If no restrictions apply: Show

### 6.3 Implementation Details

#### 6.3.1 Visibility Function
- Function: shouldExposeAvatar (duplicated in contact_service.go and chat_service.go)
- Parameters: Six boolean flags for global restrictions, exemptions, and user restrictions
- Returns: Boolean indicating whether to show avatar
- Used in: GetContacts, GetContactRequests, GetUserChats

#### 6.3.2 Database Query Pattern
All contact and chat queries use identical LEFT JOIN pattern:
- Join user_global_restrictions on contact user ID
- Join user_global_restriction_exemptions checking if viewer is exempted
- Join user_restrictions checking if viewer is restricted
- All restriction flags default to FALSE via COALESCE if no row exists

#### 6.3.3 Privacy Application Direction
- Privacy restrictions apply to the contact being viewed, not the viewer
- Viewer's ID is only used to check if they are exempted or restricted
- Direction of contact relationship does not affect privacy logic

### 6.4 Automatic Exemption Cleanup
- Database trigger: trg_clean_global_restrictions
- Fires AFTER UPDATE on user_global_restrictions table
- Activates only when restrictions are lifted (TRUE → FALSE)
- Function: clean_global_restriction_exemptions()
- Behavior:
  - Sets exemption flags to FALSE for lifted restrictions
  - Deletes exemption records where all flags are FALSE
  - Ensures no orphaned exemptions remain

### 6.5 Frontend Privacy Handling
- Component: PrivacyAvatar
- Accepts uri parameter (string or null)
- If uri is null: Shows colored placeholder with initials
- If uri exists: Shows actual avatar image
- No client-side privacy logic (backend controls uri presence)
- Placeholder color: Deterministic hash based on name
- Backend returns null for restricted avatars

### 6.6 Scope Clarification

#### 6.6.1 Visibility vs. Messaging
- Profile privacy restrictions govern visibility of profile information, avatars, and status
- Messaging permissions are governed separately by:
  - Contact relationships
  - Blocking rules
  - Profile type
  - Primary device rules

#### 6.6.2 Future Considerations
- If a messaging permission setting is added, it shall be implemented as a separate messaging-specific field
- Visibility restriction fields shall not be reused for messaging permission logic

---

## 7. Primary Device (Central Session)

### 7.1 Database Constraint
- The system shall enforce exactly one primary device session per user
- This constraint is maintained through a unique partial database index on auth_user_id where is_central = TRUE
- Index ensures only one session per user can have is_central flag set to TRUE

### 7.2 Set Primary Device
- Process:
  1. System shall deactivate all existing primary device designations for the user
  2. System shall designate the current session as the primary device

### 7.3 Primary Device Requirements

#### 7.3.1 Platform Restriction
- The primary device must be a native mobile platform (iOS or Android)
- Web sessions cannot be designated as primary device
- Platform values: 'ios', 'android', or 'web'

#### 7.3.2 Device Switching Rules
- Switching primary device should require the old primary device to be online
- If the old primary device is offline:
  - System shall prompt the user that data may be lost
  - System shall not allow the switch without explicit user confirmation
  - Default behavior: Reject the switch request

---

## Appendix: Database Schema

### 7.1 Tables

#### 7.1.1 users
Stores user profile information.

**Columns:**
- `id` (UUID, Primary Key) - User identifier, references auth_users
- `name` (TEXT, NOT NULL) - User display name, max 40 characters
- `bio` (TEXT) - User biography, max 150 characters
- `profile_type` (TEXT, NOT NULL) - Profile visibility type: 'public', 'private', or 'personal'
- `is_admin_blocked` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Administrative block status
- `admin_block_reason` (TEXT) - Reason for administrative block
- `hmac_sha256_hex_username` (TEXT, NOT NULL, UNIQUE) - HMAC-SHA256 hash of username, exactly 64 characters
- `b64_cipher_chacha20poly1305_username` (TEXT, NOT NULL) - ChaCha20-Poly1305 encrypted username, max 52 characters
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Foreign key to auth_users(id) with CASCADE delete
- Profile type must be one of: 'public', 'private', 'personal'
- HMAC username must be exactly 64 characters (CHECK constraint)
- Encrypted username must be max 52 characters (CHECK constraint)
- Name must be max 40 characters (CHECK constraint)
- Bio must be max 150 characters (CHECK constraint)

**Indexes:**
- Primary key on `id`
- Unique index on `hmac_sha256_hex_username`
- Composite index on `(profile_type, is_admin_blocked)` where `is_admin_blocked = FALSE`
- Index on `id` where `is_admin_blocked = TRUE`
- Composite index on `(profile_type, created_at DESC)` where `is_admin_blocked = FALSE`

**Triggers:**
- `users_timestamps_trigger` - Automatic timestamp management via set_timestamps()

---

#### 7.1.2 alone_username
Stores plaintext usernames with random identifiers unlinked to user accounts.

**Purpose:**
- Enables username availability checks during registration
- Provides plaintext username access for system operations
- Uses random UUID identifiers with no direct relationship to user accounts

**Columns:**
- `id` (UUID, Primary Key) - Random identifier (not linked to user ID)
- `username` (TEXT, NOT NULL, UNIQUE) - Plaintext username, exactly 10 characters
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `username`
- Username must be exactly 10 characters (CHECK constraint)

**Indexes:**
- Primary key on `id`
- Unique index on `username`

**Triggers:**
- `alone_username_timestamps_trigger` - Automatic timestamp management via set_timestamps()

---

#### 7.1.3 avatars
Stores user avatar information and access tokens.

**Columns:**
- `id` (UUID, Primary Key) - Avatar record identifier
- `user_id` (UUID, NOT NULL) - User identifier, references users(id)
- `file_id` (TEXT, NOT NULL) - File identifier for the avatar
- `avatar_type` (TEXT, NOT NULL, DEFAULT 'profile') - Avatar type
- `token_id` (TEXT) - Access token identifier
- `token_secret` (TEXT) - Access token secret
- `token_expiry` (TIMESTAMPTZ) - Token expiration timestamp (1 year from creation)
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `(user_id, file_id)`
- Foreign key to users(id) with CASCADE delete
- Check constraint: Profile avatars must use user_id as file_id

**Indexes:**
- Primary key on `id`
- Unique index on `(user_id, file_id)`
- Unique partial index on `(user_id, avatar_type)` where `avatar_type = 'profile'`
- Index on `(user_id, avatar_type, created_at DESC)`
- Index on `token_expiry` where `token_expiry IS NOT NULL`

**Triggers:**
- `avatars_timestamps_trigger` - Automatic timestamp management via set_timestamps()

**Token Management:**
- Tokens expire after 1 year from creation
- Refresh triggered if token is expired or expiry is invalid
- Empty file_id returns null (no avatar)

---

#### 7.1.4 sessions
Stores user session information including primary device designation.

**Columns:**
- `id` (UUID, Primary Key) - Session identifier
- `auth_user_id` (UUID, NOT NULL) - User identifier, references auth_users(id)
- `token_hash` (TEXT, NOT NULL, UNIQUE) - Hashed session token
- `device_token` (TEXT) - Firebase Cloud Messaging token
- `platform` (TEXT) - Platform type: 'ios', 'android', or 'web'
- `device_name` (TEXT) - Human-readable device name
- `is_central` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Primary device designation
- `user_agent` (TEXT) - Browser/app user agent string
- `ip_address` (TEXT) - IP address of the session
- `expires_at` (TIMESTAMPTZ, NOT NULL) - Session expiration timestamp
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `token_hash`
- Foreign key to auth_users(id) with CASCADE delete
- Platform must be one of: 'ios', 'android', or 'web'

**Indexes:**
- Primary key on `id`
- Unique index on `token_hash`
- Index on `auth_user_id`
- Unique partial index on `auth_user_id` where `is_central = TRUE` (enforces one primary device per user)

**Triggers:**
- `sessions_timestamps_trigger` - Automatic timestamp management via set_timestamps()

---

#### 7.1.5 user_contacts
Stores one-way contact relationships between users.

**Columns:**
- `owner_user_id` (UUID, NOT NULL) - User who added the contact, references users(id)
- `contact_user_id` (UUID, NOT NULL) - User being added as contact, references users(id)
- `nickname` (TEXT) - Optional nickname for the contact, max 40 characters (Unicode character count)
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Composite primary key on `(owner_user_id, contact_user_id)`
- Foreign keys to users(id) with CASCADE delete
- Nickname max 40 characters (CHECK constraint)

**Indexes:**
- Primary key creates index on `(owner_user_id, contact_user_id)`
- Index on `contact_user_id` for reverse lookups

**Triggers:**
- `user_contacts_timestamps_trigger` - Automatic timestamp management via set_timestamps()

**Nickname Handling:**
- Trimmed before storage
- Empty trimmed strings stored as NULL
- Validated using Unicode character count (not byte count)
- Frontend enforces 40 character limit in real-time

---

#### 7.1.6 contact_requests
Stores pending, accepted, and declined contact requests.

**Columns:**
- `id` (UUID, Primary Key) - Request identifier
- `requester_user_id` (UUID, NOT NULL) - User sending the request, references users(id)
- `receiver_user_id` (UUID, NOT NULL) - User receiving the request, references users(id)
- `status` (request_status_enum, NOT NULL, DEFAULT 'pending') - Request status: 'pending', 'accepted', or 'declined'
- `nickname` (TEXT) - Optional nickname for the contact, max 40 characters
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `(requester_user_id, receiver_user_id)`
- Check constraint: `requester_user_id != receiver_user_id` (no self-requests)
- Foreign keys to users(id) with CASCADE delete
- Nickname max 40 characters (CHECK constraint)

**Indexes:**
- Primary key on `id`
- Unique index on `(requester_user_id, receiver_user_id)`
- Partial index on `(receiver_user_id, created_at DESC)` including `requester_user_id` where `status = 'pending'`
- Partial index on `(requester_user_id, created_at DESC)` including `receiver_user_id` where `status = 'pending'`
- Index on `updated_at` where `status IN ('accepted', 'declined')` for cleanup

**Triggers:**
- `contact_requests_timestamps_trigger` - Automatic timestamp management via set_timestamps()
- `auto_add_contact_on_accept` - Automatically creates one-way contact when request is accepted (AFTER UPDATE OF status when status changes from 'pending' to 'accepted')

**Enums:**
- `request_status_enum` - Values: 'pending', 'accepted', 'declined'

**Functions:**
- `add_contact_on_accept()` - Inserts record into user_contacts with owner_user_id = requester_user_id and contact_user_id = receiver_user_id, copies nickname from request, uses ON CONFLICT DO NOTHING for idempotency

**Status Transitions:**
- pending → accepted: Triggers automatic contact creation
- pending → declined: No contact created
- Accepted/declined requests can be deleted and recreated as new pending requests

**Request Lifecycle:**
- No TTL or expiration on contact requests
- Requests remain indefinitely until accepted/declined/undone
- Cleanup index exists for processed requests

#### 7.1.7 user_blocks
Stores block relationships between users.

**Columns:**
- `id` (UUID, Primary Key) - Block record identifier
- `blocker_user_id` (UUID, NOT NULL) - User who initiated the block, references users(id)
- `blocked_user_id` (UUID, NOT NULL) - User being blocked, references users(id)
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `(blocker_user_id, blocked_user_id)`
- Foreign keys to users(id) with CASCADE delete

**Indexes:**
- Primary key on `id`
- Unique index on `(blocker_user_id, blocked_user_id)`
- Index on `blocked_user_id` for reverse lookups

**Triggers:**
- `user_blocks_timestamps_trigger` - Automatic timestamp management via set_timestamps()
- `auto_remove_contact_on_block` - Automatically removes mutual contacts when block is created (AFTER INSERT)

**Functions:**
- `remove_contact_on_block()` - Deletes both directional contact relationships using tuple deletion pattern: (blocker → blocked) and (blocked → blocker)

**Block Behavior:**
- Idempotent: Blocking already-blocked user returns success
- Reciprocal blocking allowed: If B blocked A, A can still block B
- Automatic contact cleanup in both directions
- Automatic pending message deletion in both directions
- Uses ON CONFLICT DO NOTHING for idempotency

---

#### 8.1.8 user_restrictions
Stores user-specific restrictions for profile, avatar, and status visibility.

**Columns:**
- `id` (UUID, Primary Key) - Restriction record identifier
- `user_id` (UUID, NOT NULL) - User setting the restriction, references users(id)
- `restricted_user_id` (UUID, NOT NULL) - User being restricted, references users(id)
- `restrict_profile` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide profile from restricted user
- `restrict_avatar` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide avatar from restricted user
- `restrict_status` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide status from restricted user
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `id`
- Unique constraint on `(user_id, restricted_user_id)`
- Foreign keys to users(id) with CASCADE delete

**Indexes:**
- Primary key on `id`
- Unique index on `(user_id, restricted_user_id)`
- Partial index on `(user_id, restricted_user_id)` where `restrict_profile = TRUE`
- Partial index on `(user_id, restricted_user_id)` where `restrict_avatar = TRUE`
- Partial index on `(user_id, restricted_user_id)` where `restrict_status = TRUE`
- Index on `restricted_user_id` for reverse lookups
- Covering index on `(user_id, restricted_user_id)` including `(restrict_profile, restrict_avatar, restrict_status)`

**Triggers:**
- `user_restrictions_timestamps_trigger` - Automatic timestamp management via set_timestamps()

**Default Behavior:**
- All restriction flags default to FALSE (permissive by default)
- No exemption mechanism for user-specific restrictions

---

#### 8.1.9 user_global_restrictions
Stores global restrictions set by a user that apply to all users except exempted ones.

**Columns:**
- `user_id` (UUID, Primary Key) - User setting the global restriction, references users(id)
- `restrict_avatar` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide avatar globally
- `restrict_status` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide status globally
- `restrict_profile` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Hide profile globally
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Primary key on `user_id`
- Foreign key to users(id) with CASCADE delete

**Indexes:**
- Primary key on `user_id`
- Partial index on `user_id` where `restrict_avatar = TRUE`
- Partial index on `user_id` where `restrict_status = TRUE`
- Partial index on `user_id` where `restrict_profile = TRUE`
- Covering index on `user_id` including `(restrict_profile, restrict_avatar, restrict_status)`

**Triggers:**
- `user_global_restrictions_timestamps_trigger` - Automatic timestamp management via set_timestamps()
- `trg_clean_global_restrictions` - Automatically cleans up exemptions when restrictions are lifted (AFTER UPDATE OF restrict_avatar, restrict_status, restrict_profile when any changes from TRUE to FALSE)

**Functions:**
- `clean_global_restriction_exemptions()` - Updates exemption flags to FALSE for lifted restrictions, deletes exemption records where all flags are FALSE

**Default Behavior:**
- All restriction flags default to FALSE (permissive by default)
- Exemptions can override global restrictions

---

#### 8.1.10 user_global_restriction_exemptions
Stores users who are exempted from owner's global restrictions.

**Columns:**
- `user_id` (UUID, NOT NULL) - Owner of the global restriction, references users(id)
- `exempted_user_id` (UUID, NOT NULL) - User exempted from restriction, references users(id)
- `exception_avatar` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Allow avatar view despite global restriction
- `exception_status` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Allow status view despite global restriction
- `exception_profile` (BOOLEAN, NOT NULL, DEFAULT FALSE) - Allow profile view despite global restriction
- `created_at` (TIMESTAMPTZ) - Record creation timestamp
- `updated_at` (TIMESTAMPTZ) - Record last update timestamp

**Constraints:**
- Composite primary key on `(user_id, exempted_user_id)`
- Foreign keys to users(id) with CASCADE delete

**Indexes:**
- Primary key creates index on `(user_id, exempted_user_id)`
- Covering index on `(user_id, exempted_user_id)` including `(exception_profile, exception_avatar, exception_status)`

**Triggers:**
- `user_global_restriction_exemptions_timestamps_trigger` - Automatic timestamp management via set_timestamps()

**Automatic Cleanup:**
- When global restrictions are lifted, corresponding exemptions are automatically set to FALSE
- Exemption records with all FALSE flags are automatically deleted

---

### 8.2 Global Functions

#### 8.2.1 set_timestamps()
Automatically sets created_at and updated_at timestamps for all tables.

**Behavior:**
- On INSERT: Sets both created_at and updated_at to current timestamp
- On UPDATE: Updates only updated_at to current timestamp

**Applied to all tables via triggers**