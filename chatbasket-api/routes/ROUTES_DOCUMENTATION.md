# ChatBasket API Routes Documentation - FINAL VERIFIED

## 📑 **Table of Contents with Line Numbers**

| Section | Line Range | Description |
|---------|------------|-------------|
| **Route Groups & Task Flows** | 62-865 | Complete user journey flows |
| ├─ Complete Onboarding Flow | 62-108 | Auth → Public Profile → Personal Profile → Avatar |
| ├─ User Authentication Flow | 110-184 | Signup → OTP verification → Login → Session |
| ├─ Complete Login Flow | 186-230 | Login → OTP verification → User info → Domain |
| ├─ Social Interaction Flow | 231-285 | Contacts → Chat → Messaging → File sharing |
| ├─ Device Management Flow | 287-345 | Multi-device → Primary device → Tokens → Logout |
| ├─ Privacy Management Flow | 346-386 | Blocking → Profile privacy → Avatar management |
| ├─ Messaging & File Sharing Flow | 387-492 | Contacts → Chat → Messages → Files → ACK |
| ├─ Authentication & Settings Flow | 493-582 | OTP updates → Primary device → Notification tokens |
| ├─ Profile Management Flow | 583-626 | Get/Update → Avatar management → Logout |
| ├─ Session Management Flow | 627-694 | Login → Primary device → Tokens → Logout |
| ├─ User Registration Flow | 695-734 | Username check → Profile creation → Avatar upload |
| ├─ Contact Management Flow | 735-786 | Existence check → Create/Request → Management |
| ├─ Chat System Flow | 787-836 | Eligibility → Chat creation → Send/Files → ACK |
| ├─ Auth & Settings Flow | 837-876 | OTP updates → Device management → Tokens |
| └─ Session Management Flow | 877-916 | Login → Primary device → Logout |
| **Authentication Routes** | 917-992 | `/auth/*` endpoints |
| **Public Routes** | 993-999 | `/public/profile/*` endpoints |
| **Common Routes** | 1000-1114 | `/common/*` endpoints |
| **Personal Routes** | 1115-1791 | `/personal/*` endpoints |

---

## Overview
Complete and final verified documentation of all API routes with actual business logic, flow, and functionality for Personal, Public, and Common domains.

---

## 📋 **Line Number Index**
```
Line 62:  🔄 Complete Onboarding Flow
Line 110: 🔐 Complete User Authentication Flow  
Line 186: 🔄 Complete Login Flow
Line 231: 🔄 Social Interaction Flow
Line 287: 🔄 Device Management Flow
Line 346: 🔄 Privacy Management Flow
Line 387: 🔄 Messaging & File Sharing Flow
Line 493: 🔄 Authentication & Settings Flow
Line 583: 👤 Complete Profile Management Flow
Line 627: 🔄 Session Management Flow
Line 695: 📱 Complete User Registration Flow
Line 735: 👥 Contact Management Flow
Line 787: 💬 Chat System Flow
Line 837: 🔐 Auth & Settings Flow
Line 877: 🔄 Session Management Flow
Line 917: 🔒 Authentication Routes (/auth/*)
Line 993: 🌐 Public Routes (/public/*)
Line 1000: 🔒 Common Routes (/common/*)
Line 1115: 👤 Personal Routes (/personal/*)
```

---

## � **Route Groups & Task Flows**

### **🔄 Complete Onboarding Flow**
**Route Groups Involved:** `/auth/*` → `/public/profile/*` → `/personal/profile/*`

```
Step 1: User Authentication (Required First)
POST /auth/signup
{
  "email": "user@example.com",
  "password": "secure_password"
}
↓
Step 2: Verify Signup OTP (Required)
POST /auth/signup-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "web|android|ios"
}
↓
Step 3: Create Public Profile (Required - Appwrite Database)
POST /public/profile/create-profile
{
  "username": "chosen_username",
  "name": "User Name",
  "profile_visible_to": "public|private"
}
↓
Step 4: Create Personal Profile (Optional - PostgreSQL)
POST /personal/profile/create-profile
{
  "name": "User Name",
  "profile_type": "personal"
}
// Note: Personal mode auto-generates username, no username field needed
↓
Step 5: Upload Avatar (Optional)
POST /personal/profile/upload-avatar
[multipart form with avatar file]
```

**VERIFIED Business Logic Order:**
- **Authentication Required**: Must complete auth before any profile operations
- **Public Profile First**: Required step in Appwrite Database
- **Personal Profile Optional**: PostgreSQL-based with auto-generated username
- **Avatar Optional**: Can be uploaded after profile creation

---

### � Complete User Authentication Flow
**Route Groups Involved:** `/auth/*`

```
Step 1: User Signup (with comprehensive validation)
POST /auth/signup
{
  "email": "user@example.com",
  "password": "secure_password"
}
// Business Logic: 
// 1. Check if user exists (verified vs unverified handling)
// 2. Hash password
// 3. Create unverified user in PostgreSQL
// 4. Send verification OTP via email
↓
Step 2: Verify Signup OTP (account activation)
POST /auth/signup-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "web|android|ios"
}
// Business Logic: 
// 1. Get user by email
// 2. Verify OTP via OTP flow
// 3. Mark email as verified
// 4. Create session via CreateSessionFlow
// 5. Set HTTP-only cookies for web platform
↓
Step 3: User Login (password validation)
POST /auth/login
{
  "email": "user@example.com",
  "password": "secure_password"
}
// Business Logic: 
// 1. Get user by email
// 2. Check email verification status
// 3. Validate password
// 4. Send login OTP via email
↓
Step 4: Verify Login OTP (session creation)
POST /auth/login-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "web|android|ios"
}
// Business Logic: 
// 1. Get user by email
// 2. Verify OTP via OTP flow
// 3. Create session via CreateSessionFlow
// 4. Set HTTP-only cookies for web platform
// 5. Return session response with primary device info
↓
Step 5: Resend OTP (if needed)
POST /auth/resend-otp
{
  "email": "user@example.com",
  "type": "signup|login"
}
// Business Logic: Resends OTP for specified type
```

**VERIFIED Business Logic Order:**
- **Signup Process**: User existence check → password hashing → unverified user creation → OTP verification
- **Account Verification**: OTP verification → email verification → session creation → cookie handling
- **Login Process**: User lookup → verification check → password validation → OTP sending
- **Session Creation**: OTP verification → session creation → platform-specific response
- **Platform Handling**: Web (cookies) vs Native (tokens) with primary device info
- **Domain Access**: Auth middleware grants access to appropriate routes
- **User Context**: Extracted and available in all protected routes

---

### 🔄 Complete Login Flow
**Route Groups Involved:** `/auth/*` → `/common/me` → Domain-specific routes

```
Step 1: User Login (password validation and OTP)
POST /auth/login
{
  "email": "user@example.com",
  "password": "secure_password"
}
// Business Logic: 
// 1. Get user by email
// 2. Check email verification status
// 3. Validate password
// 4. Send login OTP via email
↓
Step 2: Verify Login OTP (session creation)
POST /auth/login-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "web|android|ios"
}
// Business Logic: 
// 1. Get user by email
// 2. Verify OTP via OTP flow
// 3. Create session via CreateSessionFlow
// 4. Set HTTP-only cookies for web platform
// 5. Return session response with primary device info
↓
Step 3: Get User Info (session validation)
GET /common/me
// Business Logic: Retrieves user and session details via GetUserWithSession
↓
Step 4: Access Domain-Specific Routes
- Public Mode: /public/profile/*
- Personal Mode: /personal/* (contacts, chat, settings)
```

**VERIFIED Business Logic Order:**
- **Login Process**: User lookup → verification check → password validation → OTP sending
- **Session Creation**: OTP verification → session creation → platform-specific response
- **Session Validation**: PostgreSQL-based session storage with GetUserWithSession
- **Domain Access**: Auth middleware grants access to appropriate routes

---

### 🔄 Complete Social Interaction Flow
### **🔄 Complete Social Interaction Flow**
**Route Groups Involved:** `/personal/contacts/*` → `/personal/chat/*`

```
Step 1: Check if User Exists (HMAC-based lookup)
POST /personal/contacts/check-existence
{
  "username": "target_username"
}
↓
Step 2: Create Contact (with validation checks)
POST /personal/contacts/create
{
  "contact_user_id": "user-uuid",
  "nickname": "Optional nickname"
}
// Business Logic: Validates admin blocks, mutual blocks, profile types
↓
Step 3: Handle Contact Requests (if target profile_type=personal)
GET /personal/contacts/requests/get
↓
Step 4: Accept/Reject Requests
POST /personal/contacts/requests/accept
{
  "contact_user_id": "requester-uuid"
}
↓
Step 5: Check Messaging Eligibility (before chat)
POST /personal/chat/check-eligibility
{
  "recipient_id": "user-uuid"
}
// Business Logic: Verifies contact relationship, blocks, primary devices
↓
Step 6: Create Chat (if eligible)
POST /personal/chat/create
{
  "recipient_id": "user-uuid"
}
↓
Step 7: Send Messages (with eligibility check)
POST /personal/chat/send
{
  "recipient_id": "user-uuid",
  "content": "Hello!",
  "message_type": "text"
}
// Business Logic: Checks eligibility, creates chat, creates message with TTL
```

**VERIFIED Business Logic Order:**
- **Contact Discovery**: HMAC-based user lookup
- **Contact Creation**: Profile type-based logic (public/direct, personal/request, private/forbidden)
- **Eligibility Verification**: Required before any messaging
- **Chat Creation**: Automatic if eligible
- **Message Sending**: Includes eligibility check, chat creation, TTL management

---

### 🔄 **Complete Device Management Flow**
**Route Groups Involved:** `/auth/*` → `/personal/settings/*` → `/common/logout`

```
Step 1: User Login (multiple devices)
POST /auth/login
{
  "email": "user@example.com",
  "password": "secure_password"
}
↓
Step 2: Verify Login OTP for Each Device
POST /auth/login-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "android|ios|web"
}
↓
Step 3: Set Primary Device (native only - with validation)
POST /personal/settings/session/central
{
  "session_token": "current_session_token"
}
// Business Logic: 
// 1. Compute Token Hash
// 2. Get session details to validate platform
// 3. Validate platform != "web" (web devices blocked)
// 4. Reset all sessions to non-central
// 5. Set specific session as central
↓
Step 4: Update Notification Tokens (with platform mapping)
POST /personal/settings/session/notification-token
{
  "session_token": "current_session_token",
  "type": "fcm|apn",
  "token": "notification_token"
}
// Business Logic: Maps fcm→android, apn→ios
↓
Step 5: Logout from Specific Device
POST /common/logout
{
  "all_sessions": false
}
// Business Logic: PostgreSQL session deletion + token cleanup
```

**VERIFIED Business Logic Order:**
- **Multi-Device Support**: Multiple sessions per user
- **Primary Device Validation**: Platform check (web devices blocked), session reset, central assignment
- **Notification Token Mapping**: fcm→android, apn→ios
- **Session Control**: Granular logout with PostgreSQL cleanup

---

### **🔄 Complete Privacy Management Flow**
**Route Groups Involved:** `/personal/contacts/*` → `/personal/profile/*`

```
Step 1: Block User (with cleanup)
POST /personal/contacts/block
{
  "contact_user_id": "user-uuid"
}
// Business Logic: 
// 1. Create block record in database
// 2. Drop pending messages between users (both directions)
// 3. Automatic contact removal via database trigger
↓
Step 2: Check Current Privacy Settings
GET /personal/profile/get-profile
↓
Step 3: Update Profile Privacy
POST /personal/profile/update-profile
{
  "profile_type": "private",
  "name": "User Name",
  "bio": "Private bio"
}
// Business Logic: Updates name, bio, profile_type fields
↓
Step 4: Remove Avatar (for privacy)
DELETE /personal/profile/remove-avatar
// Business Logic: 
// 1. Check avatar existence in database and storage
// 2. List and delete all file tokens
// 3. Delete file from Appwrite storage
// 4. Delete avatar record from database
↓
Step 5: Check Contact Visibility
GET /personal/contacts/get
// Business Logic: Blocked users won't appear in contact list
```

**VERIFIED Business Logic Order:**
- **Blocking System**: Creates block record + drops pending messages + automatic contact removal
- **Avatar Management**: Complete cleanup with token deletion and storage cleanup
- **Profile Privacy**: Profile type changes affect contact visibility
- **Contact Filtering**: Blocked users excluded from contact lists

---

### 🔄 **Complete User Registration Flow**
**Route Groups Involved:** `/public/profile/*` → `/personal/profile/*`

```
Step 1: Check Username Availability
POST /public/profile/check-username
{
  "username": "desired_username"
}
↓
Step 2: Create Public Profile (Appwrite Database)
POST /public/profile/create-profile
{
  "username": "chosen_username",
  "name": "User Name",
  "bio": "Optional bio",
  "profile_visible_to": "public|private"
}
↓
Step 3: Create Personal Profile (PostgreSQL - if user wants personal mode)
POST /personal/profile/create-profile
{
  "name": "User Name", 
  "profile_type": "personal"
}
// Note: Personal mode auto-generates username, no username field needed
↓
Step 4: Upload Avatar (optional)
POST /personal/profile/upload-avatar
[multipart form with avatar file]
```

**VERIFIED Flow Details:**
- **Public Mode**: Uses provided username, stores in Appwrite Database
- **Personal Mode**: Auto-generates random username, encrypts and stores in PostgreSQL
- **Profile Types**: `public`, `personal`, `private` (private profiles cannot be contacted)

### **👥 Complete Contact Management Flow**
**Route Groups Involved:** `/personal/contacts/*`

```
Step 1: Check if User Exists (HMAC-based lookup)
POST /personal/contacts/check-existence
{
  "username": "target_username"
}
↓
Step 2: Create Contact (based on target's profile type)
POST /personal/contacts/create
{
  "contact_user_id": "user-uuid",
  "nickname": "Optional nickname"
}
↓
Step 3: Handle Contact Request (if target profile_type=personal)
- Accept: POST /personal/contacts/requests/accept
{
  "contact_user_id": "requester-uuid"
}
- Reject: POST /personal/contacts/requests/reject
{
  "contact_user_id": "requester-uuid"
}
- Undo: POST /personal/contacts/requests/undo
{
  "contact_user_id": "requester-uuid"
}
↓
Step 4: Get All Contacts
GET /personal/contacts/get
↓
Step 5: Manage Contact Details
- Update Nickname: POST /personal/contacts/update-nickname
{
  "contact_user_id": "user-uuid",
  "nickname": "New nickname"
}
- Remove Nickname: POST /personal/contacts/remove-nickname
{
  "contact_user_id": "user-uuid"
}
- Block User: POST /personal/contacts/block
{
  "contact_user_id": "user-uuid"
}
- Delete Contact: POST /personal/contacts/delete
{
  "contact_user_id": ["user-uuid-1", "user-uuid-2"]
}
```

**VERIFIED Flow Details:**
- **Public Profile**: Direct contact creation (no request needed)
- **Personal Profile**: Contact request if target doesn't have you as contact
- **Private Profile**: Forbidden - cannot be contacted
- **Bulk Operations**: Delete contacts supports multiple user IDs

### **� Complete Messaging & File Sharing Flow**
**Route Groups Involved:** `/personal/contacts/*` → `/personal/chat/*`

```
Step 1: Check Contact Relationship
GET /personal/contacts/get
↓
Step 2: Check Messaging Eligibility (required before any messaging)
POST /personal/chat/check-eligibility
{
  "recipient_id": "user-uuid"
}
// Business Logic: 
// 1. Check contact relationship via CanSendMessage query
// 2. Verify no blocks in either direction
// 3. Check both users have primary devices
↓
Step 3: Create Chat (if eligible)
POST /personal/chat/create
{
  "recipient_id": "user-uuid"
}
↓
Step 4: Send Text Message (with eligibility check)
POST /personal/chat/send
{
  "recipient_id": "user-uuid",
  "content": "Hello!",
  "message_type": "text"
}
// Business Logic: 
// 1. Check eligibility again
// 2. Create or get chat
// 3. Create message with TTL (30 days)
↓
Step 5: Send File Message (with comprehensive validation)
POST /personal/chat/upload
[multipart form: recipient_id, message_type, caption, file]
// Business Logic: 
// 1. Validate file type and size
// 2. Check eligibility
// 3. Upload to Appwrite Chat Files bucket
// 4. Create 1-year file token
// 5. Create message with file metadata
// 6. Auto-creates chat if needed
↓
Step 6: Get Chat History
GET /personal/chat/messages?chat_id=chat-uuid&limit=50&offset=0
// Business Logic: Validates chat participation, applies pagination
↓
Step 7: Get File Download URL (with token refresh)
GET /personal/chat/file-url
{
  "message_id": "message-uuid"
}
// Business Logic: 
// 1. Validate message and participation
// 2. Check token expiry (1-year default)
// 3. Refresh token if expired (delete → create → update)
// 4. Build secure download URL
↓
Step 8: Acknowledge Message Delivery (temporary relay system)
POST /personal/chat/ack
{
  "message_id": "message-uuid",
  "acknowledged_by": "recipient|sender",
  "success": true
}
// Business Logic: 
// 1. recipient = mark as delivered
// 2. sender = mark as synced to primary device
// 3. Delete message if both ACKs received
// 4. Cleanup message files
↓
Step 9: Get All Chats
GET /personal/chat/list
// Business Logic: Formats chat responses with participant info
↓
Step 10: Manage Contact Details (integrated with messaging)
- Update Nickname: POST /personal/contacts/update-nickname
- Block User: POST /personal/contacts/block (drops messages)
- Delete Contact: POST /personal/contacts/delete
```

**VERIFIED Business Logic Order:**
- **Eligibility Verification**: Required before any messaging, checks contacts, blocks, primary devices
- **Message Creation**: Includes eligibility check, chat creation, TTL management (30 days)
- **File Management**: Upload with validation, 1-year tokens, secure URLs, token refresh
- **Delivery System**: Temporary relay with ACK confirmation, automatic cleanup
- **Contact Integration**: Blocking drops messages, contact management integrated with messaging

### **� Complete Authentication & Settings Flow**
**Route Groups Involved:** `/common/settings/*` + `/personal/settings/*`

```
Step 1: Request Update OTP (with comprehensive validation)
POST /common/settings/update/request
{
  "update_type": "password|email",
  "new_value": "new_value"
}
// Business Logic: 
// 1. Get user from database
// 2. Generate OTP (6-digit)
// 3. Hash OTP
// 4. Generate update_id
// 5. Store verification code with update_id
// 6. Send OTP email (3-minute validity)
↓
Step 2: Confirm Update (OTP verification)
- Password: POST /common/settings/password/confirm
{
  "otp": "123456",
  "new_password": "new_password",
  "update_id": "uuid-from-step-1"
}
- Email: POST /common/settings/email/confirm
{
  "otp": "123456",
  "new_email": "new_email@example.com",
  "update_id": "uuid-from-step-1"
}
// Business Logic: 
// 1. Parse update_id
// 2. Get verification code by user ID and type
// 3. Verify update_id matches
// 4. Check expiry (3 minutes)
// 5. Verify OTP hash
// 6. For password: Hash new password and update
// 7. For email: Validate password first, then update email
// 8. Delete verification code
↓
Step 3: Set Primary Device (native only - with validation)
POST /personal/settings/session/central
{
  "session_token": "current_session_token"
}
// Business Logic: 5-step validation process
↓
Step 4: Update Notification Tokens (with platform mapping)
POST /personal/settings/session/notification-token
{
  "session_token": "current_session_token",
  "type": "fcm|apn",
  "token": "notification_token"
}
// Business Logic: Maps fcm→android, apn→ios
↓
Step 5: Logout Options
- Single session: POST /public/profile/logout
{
  "all_sessions": false
}
- All sessions: POST /public/profile/logout
{
  "all_sessions": true
}
- Common logout: POST /common/logout
{
  "all_sessions": true|false
}
// Business Logic: PostgreSQL session deletion + token cleanup
```

**VERIFIED Business Logic Order:**
- **OTP System**: 6-step process with update_id tracking, 3-minute expiry
- **Password Update**: OTP verification → password hashing → database update
- **Email Update**: Password verification → email existence check → database update
- **Primary Device**: 5-step validation (hash → session → platform → reset → set)
- **Session Management**: PostgreSQL-based with token cleanup

### **👤 Complete Profile Management Flow**
**Route Groups Involved:** `/public/profile/*` + `/personal/profile/*`

```
Step 1: Get Current Profile (with context extraction)
- Public: GET /public/profile/get-profile
// Business Logic: Retrieves user from Appwrite Database
- Personal: GET /personal/profile/get-profile
// Business Logic: Retrieves from PostgreSQL + decrypts username
↓
Step 2: Update Profile (field-level updates)
- Public: POST /public/profile/update-profile
{
  "name": "Updated Name",
  "bio": "Updated bio",
  "profile_visible_to": "public|private"
}
// Business Logic: Updates fields in Appwrite Database
- Personal: POST /personal/profile/update-profile
{
  "name": "Updated Name",
  "bio": "Updated bio",
  "profile_type": "public|personal|private"
}
// Business Logic: Updates name, bio, profile_type fields in PostgreSQL
↓
Step 3: Manage Avatar (comprehensive file handling)
- Upload: POST /personal/profile/upload-avatar
[multipart form with avatar file]
// Business Logic: 
// 1. Validate multipart form and file size (5MB limit)
// 2. Check existing avatar in database and storage
// 3. Delete existing avatar if found
// 4. Upload to Appwrite Personal Profile Pic bucket
// 5. Create 1-year file token
// 6. Create or update avatar record
- Remove: DELETE /personal/profile/remove-avatar
// Business Logic: 
// 1. Check avatar existence in database and storage
// 2. List and delete all file tokens
// 3. Delete file from Appwrite storage
// 4. Delete avatar record from database
↓
Step 4: Logout (end session)
POST /public/profile/logout or POST /common/logout
// Business Logic: PostgreSQL session deletion + token cleanup
```

**VERIFIED Business Logic Order:**
- **Profile Retrieval**: Appwrite vs PostgreSQL with username decryption for personal
- **Profile Updates**: Field-level updates in respective databases
- **Avatar Management**: Complete file handling with validation, token management, cleanup
- **Session Management**: PostgreSQL-based session termination

### **🔄 Complete Session Management Flow**
**Route Groups Involved:** `/auth/*` → `/common/*` → `/personal/settings/*`

```
Step 1: User Authentication (multiple device support)
POST /auth/login
{
  "email": "user@example.com",
  "password": "secure_password"
}
// Business Logic: Creates session, sends OTP, returns user info
↓
Step 2: Verify Login OTP (platform-specific handling)
POST /auth/login-verification
{
  "email": "user@example.com",
  "secret": "otp_code",
  "platform": "web|android|ios"
}
// Business Logic: 
// 1. Verify OTP
// 2. Set HTTP-only cookies for web platform
// 3. Return session tokens for native platforms
↓
Step 3: Get User Info (session validation)
GET /common/me
// Business Logic: Retrieves user and session details via GetUserWithSession
↓
Step 4: Set Primary Device (native only - comprehensive validation)
POST /personal/settings/session/central
{
  "session_token": "current_session_token"
}
// Business Logic: 5-step process (hash → session → platform → reset → set)
↓
Step 5: Update Notification Tokens (platform mapping)
POST /personal/settings/session/notification-token
{
  "session_token": "current_session_token",
  "type": "fcm|apn",
  "token": "notification_token"
}
// Business Logic: Maps fcm→android, apn→ios
↓
Step 6: Logout Options (mode-specific session control)
- Single session: POST /public/profile/logout
{
  "all_sessions": false
}
// Business Logic: Delete single session from Appwrite
- All sessions: POST /public/profile/logout
{
  "all_sessions": true
}
// Business Logic: Delete all user sessions from Appwrite
- Common logout: POST /common/logout
{
  "all_sessions": true|false
}
// Business Logic: PostgreSQL deletion + FCM/APN token cleanup
```

**VERIFIED Business Logic Order:**
- **Multi-Device Support**: Multiple sessions per user with platform-specific handling
- **Session Validation**: PostgreSQL-based session storage with GetUserWithSession
- **Primary Device Management**: 5-step validation with platform restrictions
- **Notification Tokens**: Platform mapping with fcm/apn support
- **Mode-Specific Logout**: Public mode uses Appwrite, Common mode uses PostgreSQL

---

## � **Authentication Routes** (`/auth/*`)

### **Authentication**
No authentication middleware required for signup/login (public endpoints).

---

### **🔑 User Authentication**

#### **POST /auth/signup**
**Handler:** `UserHandler.Signup`  
**Purpose:** User registration with email and password  
**VERIFIED Business Logic:**
- Validates email and password fields
- Creates user account via auth service
- Sends OTP for email verification
- Returns user information (without sensitive data)
- Supports both web and native platforms

**VERIFIED Flow:**
1. Parse signup payload (email, password)
2. Validate required fields
3. Create user account
4. Send OTP for verification
5. Return user info

---

#### **POST /auth/signup-verification**
**Handler:** `UserHandler.AccountVerification`  
**Purpose:** Verify email OTP after signup  
**VERIFIED Business Logic:**
- Validates OTP, email, secret, and platform fields
- Verifies OTP via auth service
- **VERIFIED**: Sets HTTP-only cookies for web platform
- **VERIFIED**: Returns sanitized session response for web
- Returns session information for native platforms

**VERIFIED Flow:**
1. Parse verification payload
2. Validate required fields
3. Verify OTP
4. Set cookies (web only)
5. Return session response

---

#### **POST /auth/login**
**Handler:** `UserHandler.Login`  
**Purpose:** User login with email and password  
**VERIFIED Business Logic:**
- Validates email and password fields
- Authenticates user via auth service
- Sends OTP for login verification
- Returns user information (without sensitive data)
- Supports both web and native platforms

**VERIFIED Flow:**
1. Parse login payload (email, password)
2. Validate required fields
3. Authenticate user
4. Send OTP for verification
5. Return user info

---

#### **POST /auth/login-verification**
**Handler:** `UserHandler.LoginVerification`  
**Purpose:** Verify OTP for login  
**VERIFIED Business Logic:**
- Validates OTP, email, secret, and platform fields
- Verifies OTP via auth service
- **VERIFIED**: Sets HTTP-only cookies for web platform
- **VERIFIED**: Returns sanitized session response for web
- Returns session information for native platforms

**VERIFIED Flow:**
1. Parse verification payload
2. Validate required fields
3. Verify OTP
4. Set cookies (web only)
5. Return session response

---

#### **POST /auth/resend-otp**
**Handler:** `UserHandler.ResendOTP`  
**Purpose:** Resend OTP for signup or login  
**VERIFIED Business Logic:**
- Validates email and type fields
- Resends OTP via auth service
- Supports both signup and login OTP types
- Returns response with OTP status

**VERIFIED Flow:**
1. Parse resend OTP payload
2. Validate required fields
3. Resend OTP
4. Return response

---

## 🌐 **Public Routes** (`/public/*`)

### **Authentication**
All public routes require `AuthSessionMiddleware` authentication.

---

### **👤 Profile Management**

#### **POST /public/profile/logout**
**Handler:** `ProfileHandler.Logout`  
**Purpose:** User logout from current or all sessions  
**VERIFIED Business Logic:**
- Extracts `userId`, `sessionId`, `platform` from context (safe type assertion)
- Parses `all_sessions` from request payload
- **CORRECTED**: If `all_sessions=true`: Delete from Appwrite (NOT PostgreSQL)
- **CORRECTED**: If `all_sessions=false`: Delete from Appwrite (NOT PostgreSQL)
- Returns success response

**VERIFIED Flow:**
1. Extract `userId`, `sessionId` from context (safe type assertion)
2. Parse `all_sessions` from payload
3. Execute session deletion from Appwrite (both cases)
4. Return success response

---

#### **POST /public/profile/check-username**
**Handler:** `ProfileHandler.CheckIfUserNameAvailable`  
**Purpose:** Check if username is available for registration  
**VERIFIED Business Logic:**
- Uses Appwrite Database query with `query.Equal("username", payload.Username)`
- Direct username lookup (NOT HMAC-based in public mode)
- Returns availability status based on query results

**VERIFIED Flow:**
1. Extract username from request payload
2. Query Appwrite Database directly with username
3. Check if user exists
4. Return availability status

---
3. Return user profile

---

#### **POST /public/profile/upload-avatar**
**Handler:** `ProfileHandler.UploadProfilePicture`  
**Purpose:** Upload user's profile picture to Appwrite storage  
**VERIFIED Business Logic:**
- **VERIFIED**: Uploads file to Appwrite Public Profile Pic bucket
- **VERIFIED**: Generates file access token
- **VERIFIED**: Updates avatar field in Appwrite Database
- Returns file URL with token

**VERIFIED Flow:**
1. Extract file from form
2. Validate file
3. Upload to Appwrite Public bucket
4. Create file token
5. Update user avatar in database
6. Return file URL

---

#### **DELETE /public/profile/remove-avatar**
**Handler:** `ProfileHandler.RemoveProfilePicture`  
**Purpose:** Remove user's profile picture  
**VERIFIED Business Logic:**
- **VERIFIED**: Deletes file from Appwrite storage
- **VERIFIED**: Deletes file access token
- **VERIFIED**: Updates user avatar field to null in database
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context
2. Get current avatar information
3. Delete file from Appwrite storage
4. Delete file token
5. Update user avatar field to null
6. Return success response to null

---

#### **POST /public/profile/update-profile**
**Handler:** `ProfileHandler.UpdateProfile`  
**Purpose:** Update user's profile information  
**VERIFIED Business Logic:**
- Updates user profile in Appwrite Database
- Validates update payload
- Returns updated profile information

**VERIFIED Flow:**
1. Extract `userId` from context
2. Parse profile update payload
3. Update profile in Appwrite Database
4. Return updated profile

---

---

## 🔒 **Common Routes** (`/common/*`)

### **Authentication**
All common routes require `AuthSessionMiddleware` authentication.

---

### **⚙️ Settings Management**

#### **POST /common/settings/update/request**
**Handler:** `SettingHandler.RequestUpdateOTP`  
**Purpose:** Request OTP for profile updates  
**VERIFIED Business Logic:**
- Validates update request payload
- Generates OTP for update operation
- Sends OTP via email service
- Stores OTP request in database
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context (safe type assertion)
2. Parse update request payload
3. Generate OTP
4. Send OTP via email
5. Store OTP request
6. Return success response

---

#### **POST /common/settings/password/confirm**
**Handler:** `SettingHandler.ConfirmPasswordUpdate`  
**Purpose:** Confirm password update with OTP  
**VERIFIED Business Logic:**
- Validates OTP and new password
- Verifies OTP from database
- Hashes new password
- Updates user password in auth database
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context
2. Parse password update payload
3. Verify OTP
4. Hash new password
5. Update password in auth database
6. Return success response

---

#### **POST /common/settings/email/request**
**Handler:** `SettingHandler.RequestEmailUpdate`  
**Purpose:** Request email update with OTP  
**VERIFIED Business Logic:**
- Validates email update request
- Generates OTP for email update
- Sends OTP to new email address
- Stores email update request
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context
2. Parse email update payload
3. Generate OTP
4. Send OTP to new email
5. Store email update request
6. Return success response

---

#### **POST /common/settings/email/confirm**
**Handler:** `SettingHandler.ConfirmEmailUpdate`  
**Purpose:** Confirm email update with OTP  
**VERIFIED Business Logic:**
- Validates OTP and new email
- Verifies OTP from database
- Updates user email in auth database
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context
2. Parse email confirmation payload
3. Verify OTP
4. Update email in auth database
5. Return success response

---

### **🔐 Authentication**

#### **POST /common/logout**
**Handler:** `AuthHandler.Logout`  
**Purpose:** Common logout endpoint  
**VERIFIED Business Logic:**
- **CORRECTED**: Uses PostgreSQL for session deletion (both single and all sessions)
- Supports session-specific or all-session logout
- **ADDITIONAL**: Deletes FCM/APN tokens for personal mode users
- Returns success response

**VERIFIED Flow:**
1. Extract `userId`, `sessionId` from context
2. Parse `all_sessions` from payload
3. Execute session deletion from PostgreSQL
4. **ADDITIONAL**: Delete user tokens if in personal mode
5. Return success response

---

#### **GET /common/me**
**Handler:** `AuthHandler.GetUser`  
**Purpose:** Get current user information  
**VERIFIED Business Logic:**
- Retrieves user information from auth database
- Returns user profile with authentication details

**VERIFIED Flow:**
1. Extract `userId` from context
2. Query user information from auth database
3. Return user profile

---

---

## 👥 **Personal Routes** (`/personal/*`)

### **Authentication**
All personal routes require `AuthSessionMiddleware` authentication.

---

### **👥 Contact Management**

#### **GET /personal/contacts/get**
**Handler:** `ContactHandler.GetContacts`  
**Purpose:** Get user's contacts and people who added user  
**VERIFIED Business Logic:**
- Retrieves contacts user added via `GetUserContacts` query
- Retrieves users who added current user via `GetUsersWhoAddedYou` query
- Applies privacy restrictions for avatar visibility via `shouldExposeAvatar` function
- Decrypts usernames using `DecryptUsername` with `PersonalUsernameKey`
- Computes mutual contact status by cross-referencing both lists
- Returns both contact lists with mutual status

**VERIFIED Flow:**
1. Extract `userId` from context (safe type assertion)
2. Query user's contacts (`GetUserContacts`)
3. Query users who added user (`GetUsersWhoAddedYou`)
4. Apply privacy restrictions for avatar visibility
5. Decrypt usernames
6. Compute mutual status
7. Return contact lists

---

#### **POST /personal/contacts/check-existence**
**Handler:** `ContactHandler.CheckContactExistance`  
**Purpose:** Check if user exists by username  
**VERIFIED Business Logic:**
- Computes HMAC of username using `ComputeHMAC` with `PersonalUsernameKey`
- Queries user by hashed username via `GetUserByHashedUsername`
- Returns existence with profile type
- For private profiles, omits `recipient_user_id`
- Prevents username enumeration attacks

**VERIFIED Flow:**
1. Parse username from payload
2. Compute HMAC of username
3. Query user by hashed username
4. Return existence with profile constraints
5. Apply private profile rules

---

#### **POST /personal/contacts/create**
**Handler:** `ContactHandler.CreateContact`  
**Purpose:** Add new contact or send contact request  
**VERIFIED Business Logic:**
- Validates target user exists and prevents self-addition
- Checks admin block status via `IsUserAdminBlocked` (both directions)
- Checks mutual block status via `IsEitherBlocked` (returns 1, 2, or 0)
- **VERIFIED**: Applies profile type logic:
  - **Public**: Direct contact creation via `InsertUserContact`
  - **Personal**: Checks if target already has you via `IsAlreadyContact`, otherwise creates request via `InsertContactRequest`
  - **Private**: Rejects with forbidden error
- Handles nickname validation (max 40 characters)
- Returns appropriate response based on profile type

**VERIFIED Flow:**
1. Parse contact creation payload
2. Validate target user and prevent self-addition
3. Check admin blocks
4. Check mutual blocks
5. Apply profile type logic
6. Create contact or request
7. Return appropriate response

---

#### **POST /personal/contacts/delete**
**Handler:** `ContactHandler.DeleteContact`  
**Purpose:** Remove existing contact  
**VERIFIED Business Logic:**
- **VERIFIED**: Supports bulk deletion of multiple contacts
- **VERIFIED**: Validates and deduplicates contact IDs
- **VERIFIED**: Prevents self-deletion with `self_action_not_allowed` error
- Removes contacts from database via `DeleteUserContact`
- Returns success response

**VERIFIED Flow:**
1. Parse contact deletion payload (supports multiple IDs)
2. Validate and deduplicate contact IDs
3. Prevent self-deletion
4. Delete contacts from database
5. Return success response

---

#### **GET /personal/contacts/requests/get**
**Handler:** `ContactHandler.GetContactRequests`  
**Purpose:** Get pending contact requests  
**VERIFIED Business Logic:**
- Retrieves pending contact requests for user via `GetContactRequests`
- Returns list of requests with sender information

**VERIFIED Flow:**
1. Extract `userId` from context
2. Query pending contact requests
3. Return requests list

---

#### **POST /personal/contacts/requests/accept**
**Handler:** `ContactHandler.AcceptContactRequest`  
**Purpose:** Accept contact request  
**VERIFIED Business Logic:**
- Validates request exists and is pending
- **VERIFIED**: Prevents self-action with `self_action_not_allowed` error
- Updates request status to 'accepted' via `AcceptContactRequest`
- **VERIFIED**: Handles multiple outcomes: "accepted", "not_found", "processed"
- Creates one-way contact from requester to receiver
- Returns success response

**VERIFIED Flow:**
1. Parse acceptance payload
2. Validate request exists
3. Prevent self-action
4. Update request status
5. Handle different outcomes
6. Create one-way contact
7. Return success response

---

#### **POST /personal/contacts/requests/reject**
**Handler:** `ContactHandler.RejectContactRequest`  
**Purpose:** Reject contact request  
**VERIFIED Business Logic:**
- Validates request exists and is pending
- **VERIFIED**: Prevents self-action with `self_action_not_allowed` error
- Updates request status to 'declined' via `RejectContactRequest`
- **VERIFIED**: Handles multiple outcomes: "declined", "not_found", "processed"
- Returns success response

**VERIFIED Flow:**
1. Parse rejection payload
2. Validate request exists
3. Prevent self-action
4. Update request status
5. Handle different outcomes
6. Return success response

---

#### **POST /personal/contacts/requests/undo**
**Handler:** `ContactHandler.UndoContactRequest`  
**Purpose:** Undo sent contact request  
**VERIFIED Business Logic:**
- Validates request exists and is pending
- Deletes contact request via `UndoContactRequest`
- Returns success response

**VERIFIED Flow:**
1. Parse undo payload
2. Validate request exists
3. Delete request
4. Return success response

---

#### **POST /personal/contacts/update-nickname**
**Handler:** `ContactHandler.UpdateContactNickname`  
**Purpose:** Update contact's nickname  
**VERIFIED Business Logic:**
- Validates contact exists
- **VERIFIED**: Validates nickname length (max 40 characters)
- Updates nickname via `UpdateContactNickname`
- Returns success response

**VERIFIED Flow:**
1. Parse nickname update payload
2. Validate contact exists
3. Validate nickname (max 40 characters)
4. Update nickname
5. Return success response

---

#### **POST /personal/contacts/remove-nickname**
**Handler:** `ContactHandler.RemoveContactNickname`  
**Purpose:** Remove contact's nickname  
**VERIFIED Business Logic:**
- Validates contact exists
- Removes nickname via `RemoveContactNickname`
- Returns success response

**VERIFIED Flow:**
1. Parse nickname removal payload
2. Validate contact exists
3. Remove nickname
4. Return success response

---

#### **POST /personal/contacts/block**
**Handler:** `ContactHandler.BlockUser`  
**Purpose:** Block user  
**VERIFIED Business Logic:**
- Validates user exists and prevents self-blocking
- Creates block record via `CreateUserBlock`
- **VERIFIED**: Removes existing contacts (both directions) via database trigger
- **VERIFIED**: Drops pending messages via `DropPendingMessagesBetweenUsers`
- Returns success response

**VERIFIED Flow:**
1. Parse block payload
2. Validate user exists
3. Create block record
4. Remove contacts (automatic via trigger)
5. Drop pending messages
6. Return success response

---

### **⚙️ Settings Management**

#### **POST /personal/settings/session/central**
**Handler:** `SettingHandler.UpdateSessionCentral`  
**Purpose:** Set device as primary (central) device  
**VERIFIED Business Logic:**
- Validates session exists and belongs to user via `GetSessionByToken`
- **CRITICAL**: Validates platform is not "web" (native devices only)
- Resets all sessions to non-central via `ResetCentralSessions`
- Sets specified session as central via `SetSessionCentralByToken`
- Returns success response

**VERIFIED Flow:**
1. Parse session token from payload
2. Get session details via `GetSessionByToken`
3. **Validate platform is native (not web)**
4. Reset all sessions to non-central
5. Set current session as central
6. Return success response

---

#### **POST /personal/settings/session/notification-token**
**Handler:** `SettingHandler.UpdateSessionNotificationToken`  
**Purpose:** Update FCM/APN notification token for session  
**VERIFIED Business Logic:**
- Validates session exists
- Maps payload type to platform (fcm→android, apn→ios)
- Updates session with notification token and platform via `UpdateSessionDeviceToken`
- Returns success response

**VERIFIED Flow:**
1. Parse notification token payload
2. Validate session exists
3. Map type to platform
4. Update session token and platform
5. Return success response

---

### **💬 Chat System**

#### **POST /personal/chat/check-eligibility**
**Handler:** `ChatHandler.CheckEligibility`  
**Purpose:** Check if user can send message to recipient  
**VERIFIED Business Logic:**
- Checks messaging eligibility via `CanSendMessage` query
- Verifies recipient is in sender's contacts
- Verifies no blocks in either direction
- **VERIFIED**: Verifies both users have primary devices via `GetUserPrimarySession`
- Returns eligibility status

**VERIFIED Flow:**
1. Parse eligibility check payload
2. Check contact relationship via `CanSendMessage`
3. Check block status
4. Check primary device status for both users
5. Return eligibility result

---

#### **POST /personal/chat/create**
**Handler:** `ChatHandler.CreateChat`  
**Purpose:** Create or get existing chat between users  
**VERIFIED Business Logic:**
- Validates recipient exists
- Checks messaging eligibility
- Creates new chat via `CreateChat` or returns existing
- Returns chat information

**VERIFIED Flow:**
1. Parse chat creation payload
2. Validate recipient
3. Check eligibility
4. Create or get chat
5. Return chat information

---

#### **POST /personal/chat/send**
**Handler:** `ChatHandler.SendMessage`  
**Purpose:** Send message to recipient  
**VERIFIED Business Logic:**
- Validates recipient and message content
- **VERIFIED**: Checks messaging eligibility via `CheckMessagingEligibility`
- **VERIFIED**: Creates chat via `CreateOrGetChat` if doesn't exist
- **VERIFIED**: Creates message record with TTL (30 days) via `CreateMessage`
- **VERIFIED**: Uses `DefaultMessageTTL` constant (30 * 24 * time.Hour)
- Returns message information

**VERIFIED Flow:**
1. Parse message payload
2. Validate recipient and content
3. Check eligibility (contacts, blocks, primary devices)
4. Create or get chat
5. Create message with TTL
6. Return message information

---

#### **GET /personal/chat/messages**
**Handler:** `ChatHandler.GetMessages`  
**Purpose:** Get messages for a chat  
**VERIFIED Business Logic:**
- Validates chat exists and user is participant via `IsChatParticipant`
- **VERIFIED**: Retrieves messages with pagination via `GetChatMessages`
- **VERIFIED**: Supports limit and offset parameters
- Applies file URL generation for messages with files
- Returns paginated message list

**VERIFIED Flow:**
1. Parse message query parameters (chat_id, limit, offset)
2. Validate chat participation
3. Retrieve messages with pagination
4. Generate file URLs if needed
5. Return message list

---

#### **POST /personal/chat/ack**
**Handler:** `ChatHandler.AcknowledgeDelivery`  
**Purpose:** Acknowledge message delivery (temporary relay system)  
**VERIFIED Business Logic:**
- Validates message exists
- **VERIFIED**: Checks success flag (failed deliveries handled gracefully)
- **VERIFIED**: Updates delivery status based on `acknowledged_by`:
  - **"recipient"**: Marks as delivered via `MarkMessageDeliveredToRecipient`
  - **"sender"**: Marks as synced via `MarkMessageSyncedToSenderPrimary`
- **VERIFIED**: If both delivery and sync complete, deletes message via `DeleteMessage`
- **VERIFIED**: Cleans up message files via `CleanupMessageFile`
- Returns acknowledgment status

**VERIFIED Flow:**
1. Parse acknowledgment payload
2. Validate message exists
3. Check success flag
4. Update delivery status based on `acknowledged_by`
5. Clean up message if fully delivered
6. Return acknowledgment status

---

#### **GET /personal/chat/list**
**Handler:** `ChatHandler.GetUserChats`  
**Purpose:** Get user's chat list  
**VERIFIED Business Logic:**
- **VERIFIED**: Retrieves all chats for user via `GetUserChats`
- **VERIFIED**: Formats chat responses with participant information
- **VERIFIED**: Extracts `OtherUserID` from UUID interface
- Returns chat list with count

**VERIFIED Flow:**
1. Extract `userId` from context
2. Retrieve user's chats
3. Format chat responses (convert OtherUserID from interface)
4. Return chat list with count

---

#### **POST /personal/chat/upload**
**Handler:** `ChatHandler.UploadFileForMessage`  
**Purpose:** Upload file for message attachment  
**VERIFIED Business Logic:**
- **VERIFIED**: Extracts recipient_id from form value
- **VERIFIED**: Extracts message_type and caption from form values
- **VERIFIED**: Prevents self-messaging with "Cannot send file to yourself" error
- **VERIFIED**: Validates file type and size for message context via `validateFileType`
- **VERIFIED**: Checks messaging eligibility before upload
- **VERIFIED**: Uploads file to Appwrite Chat Files bucket
- **VERIFIED**: Creates file access token with 1-year expiry
- **VERIFIED**: Creates message record with file attachment via `CreateMessageWithFile`
- **VERIFIED**: Stores file metadata (ID, name, size, MIME type, token info)
- **VERIFIED**: Returns file URL immediately after upload

**VERIFIED Flow:**
1. Extract form values (recipient_id, message_type, caption, file)
2. Validate recipient and prevent self-messaging
3. Validate file for message context
4. Check eligibility
5. Upload to Appwrite Chat Files bucket
6. Create file token (1-year expiry)
7. Create message with file metadata
8. Generate and return file URL

---

#### **GET /personal/chat/file-url**
**Handler:** `ChatHandler.GetFileURL`  
**Purpose:** Get file URL for message attachment  
**VERIFIED Business Logic:**
- Validates message exists and user is participant (sender or recipient)
- **VERIFIED**: Checks file token expiry and refreshes if needed
- **VERIFIED**: Token refresh process: delete old token → create new token → update database
- **VERIFIED**: Builds secure download URL with file ID, token, and secret
- **VERIFIED**: Handles token cleanup on errors
- Returns secure file URL

**VERIFIED Flow:**
1. Parse file URL request
2. Validate message and participation
3. Check token expiry (1-year default)
4. Refresh token if expired (delete → create → update)
5. Build secure download URL
6. Return file URL

---

### **👤 Profile Management**

#### **GET /personal/profile/get-profile**
**Handler:** `ProfileHandler.GetProfile`  
**Purpose:** Get user's personal profile  
**VERIFIED Business Logic:**
- Retrieves user's personal profile via `GetUserProfile`
- **VERIFIED**: Decrypts username for display using `DecryptUsername`
- Returns personal profile

**VERIFIED Flow:**
1. Extract `userId` from context
2. Retrieve personal profile
3. Decrypt username
4. Return profile

---
#### **POST /personal/profile/create-profile**
**Handler:** `ProfileHandler.CreateUserProfile`  
**Purpose:** Create personal profile  
**VERIFIED Business Logic:**
- **VERIFIED**: Extracts email from context
- **VERIFIED**: Generates random username via `GenerateRandomUsername`
- **VERIFIED**: Hashes username using `ComputeHMAC`
- **VERIFIED**: Encrypts username using `EncryptUsername`
- Creates personal profile via `CreateUser`
- **VERIFIED**: Creates separate username record via `CreateAloneUsername`
- Returns created profile

**VERIFIED Flow:**
1. Extract `userId`, `email` from context
2. Parse profile creation payload
3. Generate username
4. Hash and encrypt username
5. Create profile
6. Create username record
7. Return profile

---

#### **GET /personal/profile/get-profile**
**Handler:** `ProfileHandler.GetProfile`  
**Purpose:** Get user's personal profile  
**VERIFIED Business Logic:**
- **VERIFIED**: Extracts email from context
- Retrieves user's personal profile via `GetUserProfile`
- **VERIFIED**: Decrypts username for display using `DecryptUsername`
- Returns personal profile

**VERIFIED Flow:**
1. Extract `userId`, `email` from context
2. Retrieve personal profile
3. Decrypt username
4. Return profile

---

#### **POST /personal/profile/upload-avatar**
**Handler:** `ProfileHandler.UploadProfilePicture`  
**Purpose:** Upload personal profile picture  
**VERIFIED Business Logic:**
- **VERIFIED**: Validates multipart form presence
- **VERIFIED**: Validates file size (5MB limit)
- **VERIFIED**: Provides detailed error messages with available file fields
- **VERIFIED**: Checks if avatar exists in database and storage
- **VERIFIED**: Deletes existing avatar if found
- **VERIFIED**: Uploads file to Appwrite Personal Profile Pic bucket
- **VERIFIED**: Creates file access token with 1-year expiry
- **VERIFIED**: Creates or updates avatar record in database
- Returns success response

**VERIFIED Flow:**
1. Validate multipart form presence
2. Extract and validate file (5MB limit)
3. Check existing avatar in database and storage
4. Delete existing avatar if found
5. Upload to Appwrite Personal bucket
6. Create file token (1-year expiry)
7. Create or update avatar record
8. Return success response

---

#### **DELETE /personal/profile/remove-avatar**
**Handler:** `ProfileHandler.RemoveProfilePicture`  
**Purpose:** Remove personal profile picture  
**VERIFIED Business Logic:**
- **VERIFIED**: Checks if avatar exists in database and storage
- **VERIFIED**: Returns 404 if avatar not found
- **VERIFIED**: Lists and deletes all associated file tokens
- **VERIFIED**: Deletes file from Appwrite storage
- **VERIFIED**: Deletes avatar record from database
- Returns success response

**VERIFIED Flow:**
1. Check avatar existence in database and storage
2. Return 404 if not found
3. List and delete all file tokens
4. Delete file from Appwrite storage
5. Delete avatar record from database
6. Return success response

---

#### **POST /personal/profile/update-profile**
**Handler:** `ProfileHandler.UpdateProfile`  
**Purpose:** Update personal profile  
**VERIFIED Business Logic:**
- Validates profile update payload
- **VERIFIED**: Updates name, bio, and profile_type fields
- **VERIFIED**: Uses `UpdateUserProfile` query
- Returns success response

**VERIFIED Flow:**
1. Extract `userId` from context
2. Parse update payload
3. Update profile fields (name, bio, profile_type)
4. Return success response

---

## 🎯 **Route Group Summary**

| Route Group | Routes | Purpose | Key Features |
|-------------|--------|---------|--------------|
| **🔐 User Authentication** | 5 routes | Complete auth system | Signup → OTP verification → Login → Session management |
| **🔄 Complete Onboarding** | 7 routes | Full user journey | Auth → Profile creation → Avatar setup |
| **🔄 Complete Login** | 4 routes | Session management | Login → OTP → User info → Domain access |
| **🔄 Social Interaction** | 7 routes | Full social features | Contacts → Chat → Messaging → File sharing |
| **🔄 Device Management** | 5 routes | Multi-device support | Login → Primary device → Tokens → Logout |
| **🔄 Privacy Management** | 5 routes | Privacy controls | Blocking → Profile privacy → Avatar management |
| **� Messaging & File Sharing** | 10 routes | Complete communication | Contacts → Chat → Messages → Files → ACK |
| **�📱 User Registration** | 4 routes | Profile setup | Username check → Profile creation → Avatar upload |
| **👥 Contact Management** | 11 routes | Full contact system | Existence check → Create/Request → Nickname/Block/Delete |
| **💬 Chat System** | 8 routes | Complete messaging | Eligibility → Chat creation → Send/Files → ACK |
| **🔐 Auth & Settings** | 6 routes | Security & preferences | OTP updates → Primary device → Notification tokens |
| **👤 Profile Management** | 5 routes | User profiles | Get/Update → Avatar management |
| **🔄 Session Management** | 3 routes | Session control | Login → Primary device → Logout |

---

## 📊 **Route Group Dependencies**

### **🔄 Sequential Dependencies**
```
Authentication → Registration → Contacts → Chat → Settings
     ↓              ↓           ↓        ↓        ↓
Login/Signup → Profile → Contact → Message → Device
```

**VERIFIED Business Flow Order:**
1. **Authentication** (signup/login) → **Profile Creation** (registration)
2. **Profile Creation** → **Contact Management** (find users)
3. **Contact Management** → **Chat System** (messaging)
4. **Chat System** → **Settings** (device management)

### **🔐 Authentication Dependencies**
```
All protected routes require AuthSessionMiddleware
↓
Context extraction (userId, uuidUserId, sessionId, platform)
↓
Safe type assertions with error handling
↓
Business logic execution
```

**VERIFIED Auth Flow:**
1. **Auth Routes**: No middleware (public endpoints)
2. **Protected Routes**: AuthSessionMiddleware validation
3. **Context Extraction**: Safe type assertions for user data
4. **Business Logic**: Service layer execution

### **📱 Mode-Specific Dependencies**
```
Authentication: PostgreSQL session storage
↓
Public Mode: Appwrite Database for user data
↓
Personal Mode: PostgreSQL for user data + username encryption
↓
Common Routes: Shared PostgreSQL auth operations
```

**VERIFIED Mode Flow:**
1. **Authentication**: PostgreSQL-based session management
2. **Public Mode**: Appwrite Database operations
3. **Personal Mode**: PostgreSQL + encryption operations
4. **Common Routes**: PostgreSQL auth operations

---

## 🎯 **Route Summary**

| Domain | Routes | Purpose |
|--------|--------|---------|
| **Authentication** | 5 routes | User signup, login, OTP verification |
| **Public** | 7 routes | Basic profile management (Appwrite-based) |
| **Common** | 6 routes | Shared settings and auth functionality (PostgreSQL-based) |
| **Personal** | 26 routes | Full social features (PostgreSQL-based) |
| **Total** | **44 routes** | Complete ChatBasket API functionality |

## 🔐 **Authentication**

All routes use `AuthSessionMiddleware` for authentication:
- Validates session token
- Extracts user information into context (safe type assertions)
- Handles session expiration
- Provides user context to handlers

## 📊 **Business Logic Patterns**

### **Handler-Service Separation**
- **Handlers**: HTTP request/response handling, validation, safe type assertions
- **Services**: Business logic, database operations, external API calls

### **Error Handling**
- Consistent `model.ApiError` responses
- Proper HTTP status codes
- Error type classification

### **Privacy & Security**
- HMAC-based username lookups (Personal mode only)
- Encrypted username storage (Personal mode only)
- Profile privacy restrictions
- Native device validation for primary devices
- Safe type assertions for context extraction

### **Temporary Relay Chat System**
- Messages stored temporarily with TTL (30 days)
- **VERIFIED**: Delivery acknowledgments trigger cleanup
- Primary device synchronization
- **VERIFIED**: String-based acknowledgment roles ("recipient"/"sender")
- **VERIFIED**: Success field for failed delivery handling

### **Database Architecture**
- **Public Routes**: Use Appwrite Database for user data
- **Personal Routes**: Use PostgreSQL with SQLC queries
- **Common Routes**: Use PostgreSQL for auth-related operations
- **Session Management**: PostgreSQL for all session operations

### **Key Corrections from Verification**
1. **Public logout**: Uses Appwrite (not PostgreSQL)
2. **Common logout**: Uses PostgreSQL + token cleanup
3. **Chat eligibility**: Verifies primary devices for both users
4. **Chat ACK**: String-based roles with success field
5. **Profile creation**: Generates username + creates separate record
6. **Block system**: Automatic contact removal via trigger
