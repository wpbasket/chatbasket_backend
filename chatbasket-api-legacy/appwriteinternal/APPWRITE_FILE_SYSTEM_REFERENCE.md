# Appwrite File System - Complete Reference Guide

**Project**: ChatBasket Backend  
**Date**: February 5, 2026  
**Purpose**: Comprehensive guide for Appwrite file upload, storage, and retrieval  
**Architecture Compliance**: Follows all BACKEND_CONSISTENCY.md critical standards

---

## 🏗️ Architecture Compliance

This document follows all critical backend standards:

### ✅ Handler-Service Separation Pattern
- **Handlers**: Only context validation, payload binding, service calls, response handling
- **Services**: All business logic, UUID parsing, data validation, response mapping
- **Never put business logic in handlers**

### ✅ UUID Handling Rule
- **Database**: Store as native `uuid.UUID`
- **Service Layer**: Parse from strings, work with `uuid.UUID` internally  
- **Frontend API**: Always return UUIDs as strings using `.String()` conversion

### ✅ Response Struct Rule
- **Always return custom structs** to frontend, never `map[string]interface{}`
- **Service Layer**: Return typed response structs
- **Exception**: Only use `map[string]interface{}` for dynamic wrapper objects

---

## 📚 Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Upload Flow](#upload-flow)
4. [Download Flow](#download-flow)
5. [Token Management](#token-management)
6. [URL Building Patterns](#url-building-patterns)
7. [Code Examples](#code-examples)
8. [Database Schema](#database-schema)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

---

## Overview

### What is Appwrite Storage?

Appwrite provides cloud storage for files with built-in security through token-based access control. Our implementation uses:

- **Private Buckets**: No public access, all files require tokens
- **Token-Based Access**: Each file has unique access tokens (ID + Secret)
- **Automatic Expiry**: Tokens expire after 1 year (configurable)
- **Multiple Buckets**: Separate buckets for different file types

### Current Buckets

| Bucket ID | Purpose | Max Size | Permissions |
|-----------|---------|----------|-------------|
| `personal_profile_pics` | User avatars | 10MB | API only |
| `chat_files` | Chat attachments | 100MB | API only |

---

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Application                       │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                    ┌───────────▼──────────┐
                    │   Backend API        │
                    │  (Go/Echo Server)    │
                    └───────┬──────────────┘
                            │
            ┌───────────────┼───────────────┐
            │               │               │
    ┌───────▼────────┐ ┌───▼────────┐ ┌───▼──────────┐
    │  Appwrite      │ │ PostgreSQL │ │   Utils      │
    │  Storage SDK   │ │  Database  │ │  Functions   │
    └────────────────┘ └────────────┘ └──────────────┘
            │
    ┌───────▼────────────────────────┐
    │   Appwrite Cloud Storage       │
    │  (fra.cloud.appwrite.io)       │
    └────────────────────────────────┘
```

### Data Flow

1. **Upload**: Client → Backend → Appwrite Storage
2. **Token Generation**: Backend → Appwrite Tokens API
3. **Metadata Storage**: Backend → PostgreSQL
4. **Download**: Client → Backend → Build URL → Client downloads from Appwrite

---

## Upload Flow

### Step-by-Step Process

#### 1. File Upload Request

**Client sends multipart form data:**
```http
POST /personal/chat/upload
Authorization: Bearer <session-token>
Content-Type: multipart/form-data

file: <binary-data>
recipient_id: uuid
message_type: image
caption: Optional text
```

#### 2. Backend Validation

```go
// File size check
if fileHeader.Size > MaxFileSize {
    return error("file too large")
}

// MIME type validation
mimeType := fileHeader.Header.Get("Content-Type")
if messageType == "image" && !strings.HasPrefix(mimeType, "image/") {
    return error("invalid file type")
}

// Eligibility check
eligibility := CheckMessagingEligibility(senderID, recipientID)
if eligibility != "allowed" {
    return error("messaging not allowed")
}

// Runtime safety guard: Ensure token ID/secret fields are not empty
if tokenID == "" || tokenSecret == "" {
    return error("missing token ID or secret")
}
```

#### 3. Upload to Appwrite

```go
// Generate unique file ID
fileID := uuid.New().String()

// Convert multipart file to InputFile
inputFile := file.InputFile{
    Name: fileHeader.Filename,
    Path: tempFilePath,
    Data: nil,
}

// Upload to Appwrite
uploadResult, err := storage.CreateFile(
    bucketID,    // "chat_files"
    fileID,      // Generated UUID
    inputFile,   // File data
)

// Runtime safety guard: Verify at least one token ID/secret pair was returned
if len(uploadResult.TokenIDs) == 0 || len(uploadResult.TokenSecrets) == 0 {
    // Delete uploaded file
    storage.DeleteFile(bucketID, fileID)
    return error("no token ID or secret returned")
}
```

#### 4. Generate Access Token

```go
// Set 1-year expiry
expire := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")

// Create file token
tokenResult, err := tokens.CreateFileToken(
    bucketID,
    fileID,
    tokens.WithCreateFileTokenExpire(expire),
)

// tokenResult contains:
// - Id: Token identifier
// - Secret: Token secret (used in URLs)
// - Expire: Expiration timestamp
```

#### 5. Store Metadata in PostgreSQL

```go
message, err := queries.CreateMessageWithFile(ctx, CreateMessageWithFileParams{
    ID:              messageID,
    ChatID:          chatID,
    SenderID:        senderID,
    RecipientID:     recipientID,
    Content:         caption,
    MessageType:     "image",
    FileID:          &fileID,
    FileName:        &filename,
    FileSize:        &fileSize,
    FileMimeType:    &mimeType,
    FileTokenID:     &tokenResult.Id,
    FileTokenSecret: &tokenResult.Secret,
    FileTokenExpiry: tokenExpiry,
    ExpiresAt:       messageExpiry,
})
```

#### 6. Return Response

```json
{
  "message_id": "uuid",
  "file_url": "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/{fileId}/download?project={projectId}&token={secret}",
  "file_name": "image.jpg",
  "file_size": 1024000,
  "created_at": "2026-02-05T12:00:00Z",
  "expires_at": "2026-03-07T12:00:00Z"
}
```

### Complete Upload Code Example

#### Handler Method (Follows Handler-Service Separation)
```go
func (h *ChatHandler) UploadFileForMessage(c echo.Context) error {
    // 1. Extract and validate context fields
    userId, ok := c.Get("userId").(string)
    if !ok || userId == "" {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{
            Code:    http.StatusUnauthorized,
            Message: "User id is missing or invalid",
            Type:    "unauthorized",
        })
    }
    uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
    if !okUUID {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{
            Code:    http.StatusUnauthorized,
            Message: "User id is missing or invalid",
            Type:    "unauthorized",
        })
    }

    // 2. Call service method (no business logic in handler)
    resp, apiErr := h.service.UploadFileForMessageHandler(c.Request().Context(), c, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
    if apiErr != nil {
        return c.JSON(apiErr.Code, apiErr)
    }

    // 3. Return response
    return c.JSON(http.StatusOK, resp)
}
```

#### Service Method (Contains All Business Logic)
```go
func (ps *Service) UploadFileForMessageHandler(ctx context.Context, c echo.Context, userId model.UserId) (*personalmodel.UploadFileResponse, *model.ApiError) {
    // 1. Extract and validate form data (business logic)
    recipientIDStr := c.FormValue("recipient_id")
    if recipientIDStr == "" {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "recipient_id is required",
            Type:    "invalid_request",
        }
    }

    // 2. Parse UUID (business logic)
    recipientID, err := uuid.Parse(recipientIDStr)
    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "Invalid recipient ID",
            Type:    "invalid_recipient",
        }
    }

    // 3. Business validation
    if userId.UuidUserId == recipientID {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "Cannot send file to yourself",
            Type:    "invalid_recipient",
        }
    }

    // 4. Validate eligibility (business logic)
    eligibility, err := ps.CheckMessagingEligibility(ctx, userId.UuidUserId, recipientID)
    if err != nil {
        return nil, err
    }
    if eligibility != "allowed" {
        return nil, &model.ApiError{
            Code:    http.StatusForbidden,
            Message: "messaging not allowed",
            Type:    "messaging_not_allowed",
        }
    }

    // 5. File validation (business logic)
    messageType := c.FormValue("message_type")
    if messageType == "" {
        messageType = "file"
    }

    caption := c.FormValue("caption")
    file, err := c.FormFile("file")
    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "No file provided",
            Type:    "invalid_request",
        }
    }

    if file.Size > MaxFileSize {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "file size exceeds 100MB limit",
            Type:    "file_too_large",
        }
    }

    // 6. Upload to Appwrite (business logic)
    fileID := uuid.New().String() // Generate UUID and convert to string
    uploadResult, err := ps.UploadFileFromMultipart(
        "chat_files",
        fileID,
        file,
        UploadOptions{
            DeleteExisting: false,
            GenerateTokens: true,
        },
    )

    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusInternalServerError,
            Message: "file upload failed",
            Type:    "upload_failed",
        }
    }

    // 7. Create message with file (business logic)
    message, err := ps.CreateMessageWithFile(ctx, CreateMessageWithFileParams{
        ID:              uuid.New(),
        ChatID:          chatID,
        SenderID:        userId.UuidUserId,
        RecipientID:     recipientID,
        Content:         caption,
        MessageType:     messageType,
        FileID:          &uploadResult.FileId,
        FileName:        &file.Filename,
        FileSize:        &file.Size,
        FileMimeType:    &mimeType,
        FileTokenID:     &uploadResult.TokenIDs[0],
        FileTokenSecret: &uploadResult.TokenSecrets[0],
        FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
        ExpiresAt:       pgtype.Timestamptz{Time: time.Now().Add(30*24*time.Hour), Valid: true},
    })

    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusInternalServerError,
            Message: "message creation failed",
            Type:    "message_creation_failed",
        }
    }

    // 8. Get file URL (business logic)
    fileURL, err := ps.GetMessageFileURL(ctx, message.ID, userId.UuidUserId)
    if err != nil {
        return nil, err
    }

    // 9. Return custom struct response (follows response struct rule)
    return &personalmodel.UploadFileResponse{
        MessageID: message.ID.String(), // UUID converted to string for frontend
        FileURL:   fileURL,
        FileName:  message.FileName,
        FileSize:  message.FileSize,
        CreatedAt: message.CreatedAt.Time,
        ExpiresAt: message.ExpiresAt.Time,
    }, nil
}
```

---

## Download Flow

### Step-by-Step Process

#### 1. Client Requests File URL

```http
GET /personal/chat/file-url?message_id=<uuid>
Authorization: Bearer <session-token>
```

#### 2. Backend Retrieves Message

```go
// Get message from database
message, err := queries.GetMessageByID(ctx, messageID)

// Verify user is participant
if message.SenderID != userID && message.RecipientID != userID {
    return error("forbidden")
}

// Check if file exists
if message.FileID == nil || *message.FileID == "" {
    return error("no file attached")
}
```

#### 3. Check Token Expiry

```go
now := time.Now().UTC()
needsRefresh := false

if message.FileTokenExpiry.Valid {
    // Token expired if expiry time is in the past
    needsRefresh = !message.FileTokenExpiry.Time.UTC().After(now)
} else {
    // No expiry set, needs refresh
    needsRefresh = true
}
```

#### 4. Refresh Token (if needed)

```go
if needsRefresh {
    // Generate new 1-year token
    exp := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
    
    newToken, err := tokens.CreateFileToken(
        "chat_files",
        *message.FileID,
        tokens.WithCreateFileTokenExpire(exp),
    )

    // Update database
    err = queries.UpdateMessageFileToken(ctx, UpdateMessageFileTokenParams{
        ID:              messageID,
        FileTokenID:     &newToken.Id,
        FileTokenSecret: &newToken.Secret,
        FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
    })

    tokenID = newToken.Id
    tokenSecret = newToken.Secret
} else {
    // Use existing token
    tokenID = *message.FileTokenID
    tokenSecret = *message.FileTokenSecret
}
```

#### 5. Build Download URL

```go
fileURL := utils.BuildFileDownloadURL(
    endpoint,   // "https://fra.cloud.appwrite.io/v1"
    projectID,  // "6858ed4d0005c859ea03"
    bucketID,   // "chat_files"
    &AppwriteFileData{
        FileId:     message.FileID,
        FileToken:  &tokenID,
        FileSecret: &tokenSecret,
    },
)

// Result:
// https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/{fileId}/download?project={projectId}&token={secret}
```

#### 6. Client Downloads File

```bash
# Client uses returned URL to download
curl -O "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/abc123/download?project=6858ed4d0005c859ea03&token=xyz789secret"
```

### Complete Download Code Example

#### Handler Method (Follows Handler-Service Separation)
```go
func (h *ChatHandler) GetFileURL(c echo.Context) error {
    // 1. Extract and validate context fields
    userId, ok := c.Get("userId").(string)
    if !ok || userId == "" {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{
            Code:    http.StatusUnauthorized,
            Message: "User id is missing or invalid",
            Type:    "unauthorized",
        })
    }
    uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
    if !okUUID {
        return c.JSON(http.StatusUnauthorized, &model.ApiError{
            Code:    http.StatusUnauthorized,
            Message: "User id is missing or invalid",
            Type:    "unauthorized",
        })
    }

    // 2. Bind and validate request payload
    var payload personalmodel.GetFileURLPayload
    if err := c.Bind(&payload); err != nil {
        return c.JSON(http.StatusBadRequest, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "invalid request payload",
            Type:    "bad_request",
        })
    }

    // 3. Call service method (no business logic in handler)
    resp, apiErr := h.service.GetFileURLHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
    if apiErr != nil {
        return c.JSON(apiErr.Code, apiErr)
    }

    // 4. Return response
    return c.JSON(http.StatusOK, resp)
}
```

#### Service Method (Contains All Business Logic)
```go
func (ps *Service) GetFileURLHandler(ctx context.Context, payload *personalmodel.GetFileURLPayload, userId model.UserId) (*personalmodel.GetFileURLResponse, *model.ApiError) {
    // 1. Parse UUID from payload (business logic)
    messageID, err := uuid.Parse(payload.MessageID)
    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusBadRequest,
            Message: "Invalid message ID",
            Type:    "invalid_request",
        }
    }

    // 2. Get message (business logic)
    message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
    if err != nil {
        return nil, &model.ApiError{
            Code:    http.StatusNotFound,
            Message: "message not found",
            Type:    "not_found",
        }
    }

    // 3. Verify access (business logic)
    if message.SenderID != userId.UuidUserId && message.RecipientID != userId.UuidUserId {
        return nil, &model.ApiError{
            Code:    http.StatusForbidden,
            Message: "not authorized to access this file",
            Type:    "forbidden",
        }
    }

    // 4. Check file exists (business logic)
    if message.FileID == nil || *message.FileID == "" {
        return nil, &model.ApiError{
            Code:    http.StatusNotFound,
            Message: "no file attached to this message",
            Type:    "not_found",
        }
    }

    // 5. Token refresh logic (business logic)
    now := time.Now().UTC()
    needsRefresh := false

    if message.FileTokenExpiry.Valid {
        needsRefresh = !message.FileTokenExpiry.Time.UTC().After(now)
    } else {
        needsRefresh = true
    }

    var tokenID, tokenSecret string

    if needsRefresh {
        // 6. Generate new token (business logic)
        exp := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
        newToken, err := ps.Appwrite.Tokens.CreateFileToken(
            "chat_files",
            *message.FileID,
            ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp),
        )
        if err != nil {
            return nil, &model.ApiError{
                Code:    http.StatusInternalServerError,
                Message: "failed to refresh file token",
                Type:    "internal_server_error",
            }
        }

        tokenID = newToken.Id
        tokenSecret = newToken.Secret

        // 7. Update database (business logic)
        tokenExpiry, _ := time.Parse(time.RFC3339, newToken.Expire)
        ps.PersonalQueries.UpdateMessageFileToken(ctx, UpdateMessageFileTokenParams{
            ID:              messageID,
            FileTokenID:     &tokenID,
            FileTokenSecret: &tokenSecret,
            FileTokenExpiry: pgtype.Timestamptz{Time: tokenExpiry, Valid: true},
        })
    } else {
        // 8. Use existing token (business logic)
        tokenID = *message.FileTokenID
        tokenSecret = *message.FileTokenSecret
    }

    // 9. Build download URL (business logic)
    fileURL := utils.BuildFileDownloadURL(
        ps.Appwrite.Endpoint,
        ps.Appwrite.ProjectID,
        "chat_files",
        &utils.AppwriteFileData{
            FileId:     message.FileID,
            FileToken:  &tokenID,
            FileSecret: &tokenSecret,
        },
    )

    if fileURL == nil {
        return nil, &model.ApiError{
            Code:    http.StatusInternalServerError,
            Message: "failed to build file URL",
            Type:    "internal_server_error",
        }
    }

    // 10. Return custom struct response (follows response struct rule)
    return &personalmodel.GetFileURLResponse{
        FileURL: *fileURL,
    }, nil
}
```

---

## Token Management

### Token Lifecycle

```
┌─────────────┐
│   Upload    │
│    File     │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  Generate   │
│   Token     │
│ (1yr expiry)│
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Store in  │
│ PostgreSQL  │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Use for   │
│  Downloads  │
└──────┬──────┘
       │
   ┌───┴───┐
   │       │
   ▼       ▼
┌──────┐ ┌──────────┐
│Valid │ │ Expired  │
└──────┘ └────┬─────┘
              │
              ▼
       ┌──────────────┐
       │   Refresh    │
       │    Token     │
       └──────┬───────┘
              │
              ▼
       ┌──────────────┐
       │ Update in DB │
       └──────────────┘
```

### Token Structure

```go
type Token struct {
    Id      string    // Token identifier (for reference)
    Secret  string    // Token secret (used in URLs)
    Expire  string    // ISO 8601 expiry timestamp
}
```

### Token Storage in Database

```sql
-- messages table columns
file_token_id       TEXT           -- Token ID for reference
file_token_secret   TEXT           -- Token secret for URLs
file_token_expiry   TIMESTAMPTZ    -- When token expires
```

### Token Refresh Strategy

**When to Refresh:**
- Token expiry time is in the past
- Token expiry is null
- Token is missing

**How to Refresh:**
```go
// 1. Create new token with 1-year expiry
exp := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
newToken, err := tokens.CreateFileToken(bucketID, fileID, 
    tokens.WithCreateFileTokenExpire(exp))

// 2. Update database
queries.UpdateMessageFileToken(ctx, UpdateMessageFileTokenParams{
    ID:              messageID,
    FileTokenID:     &newToken.Id,
    FileTokenSecret: &newToken.Secret,
    FileTokenExpiry: pgtype.Timestamptz{Time: expiry, Valid: true},
})

// 3. Use new token in URL
```

### Token Cleanup

**When to Delete Tokens:**
- Message is deleted (after delivery ACKs)
- File is deleted
- User deletes account

**Automatic Cleanup Logic:**
- Before deleting any file from Appwrite storage, the backend now enumerates **all** tokens associated with that file (via `Tokens.List`) and deletes each one to avoid orphaned access tokens.
- The same logic runs for generated thumbnails, ensuring no stale tokens remain for derived assets.

**How to Delete:**
```go
// Delete token from Appwrite
_, err := tokens.Delete(tokenID)

// Delete file from storage
_, err := storage.DeleteFile(bucketID, fileID)

// Delete message from database (cascade deletes metadata)
err := queries.DeleteMessage(ctx, messageID)
```

---

## URL Building Patterns

### Pattern Comparison

#### Avatar URLs (View Endpoint)
```
https://fra.cloud.appwrite.io/v1/storage/buckets/{avatarBucketId}/files/{fileId}/view?project={projectId}&token={secret}
```

**Use Case**: Inline viewing in browser (avatars, profile pictures)

#### File Downloads (Download Endpoint)
```
https://fra.cloud.appwrite.io/v1/storage/buckets/{chatFilesBucketId}/files/{fileId}/download?project={projectId}&token={secret}
```

**Use Case**: Force download (chat attachments, documents)

### URL Components

| Component | Description | Example |
|-----------|-------------|---------|
| `endpoint` | Appwrite server URL | `https://fra.cloud.appwrite.io/v1` |
| `bucketId` | Storage bucket identifier | `chat_files` |
| `fileId` | Unique file identifier | `abc123-def456` |
| `project` | Appwrite project ID | `6858ed4d0005c859ea03` |
| `token` | File access token secret | `xyz789secret` |

### Utility Functions

#### BuildAvatarURI (For Avatars)
```go
func BuildAvatarURI(ad *AppwriteFileData) *string {
    if ad == nil || ad.FileId == nil || *ad.FileId == "" || 
       ad.FileToken == nil || *ad.FileToken == "" || 
       ad.FileSecret == nil || *ad.FileSecret == "" {
        return nil
    }

    uri := fmt.Sprintf(
        "https://fra.cloud.appwrite.io/v1/storage/buckets/68f1170100025d36bf45/files/%s/view?project=6858ed4d0005c859ea03&token=%s",
        *ad.FileId, 
        *ad.FileSecret,
    )
    return &uri
}
```

#### BuildFileDownloadURL (For Chat Files)
```go
func BuildFileDownloadURL(endpoint, projectID, bucketID string, ad *AppwriteFileData) *string {
    if ad == nil || ad.FileId == nil || *ad.FileId == "" || 
       ad.FileSecret == nil || *ad.FileSecret == "" {
        return nil
    }

    uri := fmt.Sprintf(
        "%s/storage/buckets/%s/files/%s/download?project=%s&token=%s",
        endpoint, 
        bucketID, 
        *ad.FileId, 
        projectID, 
        *ad.FileSecret,
    )
    return &uri
}
```

---

## Code Examples

### Complete Upload Example

```go
package main

import (
    "fmt"
    "os"
    "time"
    
    "github.com/appwrite/sdk-for-go/appwrite"
    "github.com/appwrite/sdk-for-go/file"
    "github.com/appwrite/sdk-for-go/id"
)

func main() {
    // Initialize Appwrite client
    client := appwrite.NewClient(
        appwrite.WithEndpoint("https://fra.cloud.appwrite.io/v1"),
        appwrite.WithProject("6858ed4d0005c859ea03"),
        appwrite.WithKey("your-api-key"),
    )
    
    storage := appwrite.NewStorage(client)
    tokens := appwrite.NewTokens(client)
    
    // Create test file
    tempFile := "test.jpg"
    os.WriteFile(tempFile, []byte("test content"), 0644)
    defer os.Remove(tempFile)
    
    // Generate unique file ID
    fileID := id.Unique()
    
    // Upload file
    inputFile := file.InputFile{
        Name: tempFile,
        Path: tempFile,
        Data: nil,
    }
    
    uploadRes, err := storage.CreateFile("chat_files", fileID, inputFile)
    if err != nil {
        panic(err)
    }
    fmt.Printf("File uploaded: %s\n", uploadRes.Id)
    
    // Create token
    expire := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
    tokenRes, err := tokens.CreateFileToken(
        "chat_files",
        uploadRes.Id,
        tokens.WithCreateFileTokenExpire(expire),
    )
    if err != nil {
        panic(err)
    }
    fmt.Printf("Token created: %s\n", tokenRes.Id)
    
    // Build download URL
    downloadURL := fmt.Sprintf(
        "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/%s/download?project=6858ed4d0005c859ea03&token=%s",
        uploadRes.Id,
        tokenRes.Secret,
    )
    fmt.Printf("Download URL: %s\n", downloadURL)
}
```

### Complete Download Example

```go
func DownloadFile(messageID uuid.UUID, userID uuid.UUID) error {
    // 1. Get message
    message, err := queries.GetMessageByID(ctx, messageID)
    if err != nil {
        return err
    }
    
    // 2. Verify access
    if message.SenderID != userID && message.RecipientID != userID {
        return errors.New("forbidden")
    }
    
    // 3. Get file metadata
    fileID := *message.FileID
    
    // 4. Get or refresh token
    tokenList, err := tokens.List("chat_files", fileID)
    if err != nil {
        return err
    }
    
    var token Token
    if tokenList.Total == 0 {
        // Create new token
        exp := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
        token, err = tokens.CreateFileToken("chat_files", fileID,
            tokens.WithCreateFileTokenExpire(exp))
        if err != nil {
            return err
        }
    } else {
        token = tokenList.Tokens[0]
    }
    
    // 5. Build download URL
    downloadURL := fmt.Sprintf(
        "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/%s/download?project=6858ed4d0005c859ea03&token=%s",
        fileID,
        token.Secret,
    )
    
    // 6. Download file
    resp, err := http.Get(downloadURL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // 7. Save to disk
    out, err := os.Create("downloaded_file.jpg")
    if err != nil {
        return err
    }
    defer out.Close()
    
    _, err = io.Copy(out, resp.Body)
    return err
}
```

---

## Database Schema

### Messages Table (File-Related Columns)

```sql
CREATE TABLE messages (
    id                      UUID            PRIMARY KEY,
    chat_id                 UUID            NOT NULL,
    sender_id               UUID            NOT NULL,
    recipient_id            UUID            NOT NULL,
    content                 TEXT,
    message_type            TEXT            NOT NULL,
    
    -- File metadata
    file_id                 TEXT,
    file_name               TEXT,
    file_size               BIGINT,
    file_mime_type          TEXT,
    
    -- Token management
    file_token_id           TEXT,
    file_token_secret       TEXT,
    file_token_expiry       TIMESTAMPTZ,
    
    -- Thumbnail (optional)
    thumbnail_file_id       TEXT,
    thumbnail_token_id      TEXT,
    thumbnail_token_secret  TEXT,
    
    -- Message lifecycle
    delivered_to_recipient  BOOLEAN         DEFAULT FALSE,
    synced_to_sender_primary BOOLEAN        DEFAULT FALSE,
    expires_at              TIMESTAMPTZ     NOT NULL,
    created_at              TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ,
    
    -- Constraints
    CONSTRAINT messages_file_size_limit 
        CHECK (file_size IS NULL OR file_size <= 104857600),
    CONSTRAINT messages_file_type_validation
        CHECK (
            (message_type = 'text' AND file_id IS NULL) OR
            (message_type IN ('image', 'video', 'audio', 'file') AND file_id IS NOT NULL)
        )
);

-- Indexes
CREATE INDEX idx_messages_file_cleanup
    ON messages(file_id)
    WHERE file_id IS NOT NULL;

CREATE INDEX idx_messages_expired_file_tokens
    ON messages(file_token_expiry)
    WHERE file_id IS NOT NULL AND file_token_expiry IS NOT NULL;
```

### sqlc Queries

```sql
-- Create message with file
-- name: CreateMessageWithFile :one
INSERT INTO messages (
    id, chat_id, sender_id, recipient_id, 
    content, message_type, 
    file_id, file_name, file_size, file_mime_type,
    file_token_id, file_token_secret, file_token_expiry,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- Update file token
-- name: UpdateMessageFileToken :exec
UPDATE messages
SET file_token_id = $2,
    file_token_secret = $3,
    file_token_expiry = $4,
    updated_at = now()
WHERE id = $1;

-- Get messages with expired tokens
-- name: GetMessagesWithExpiredFileTokens :many
SELECT * FROM messages
WHERE file_id IS NOT NULL
  AND file_token_expiry IS NOT NULL
  AND file_token_expiry < now()
  AND expires_at > now()
ORDER BY created_at ASC
LIMIT $1;
```

---

## Best Practices

### 1. File Size Limits

```go
const (
    MaxAvatarSize = 10 * 1024 * 1024   // 10MB
    MaxChatFileSize = 100 * 1024 * 1024 // 100MB
)

// Always validate before upload
if fileHeader.Size > MaxChatFileSize {
    return error("file too large")
}
```

### 2. MIME Type Validation

```go
func validateFileType(fh *multipart.FileHeader, messageType string) error {
    mimeType := fh.Header.Get("Content-Type")
    
    switch messageType {
    case "image":
        if !strings.HasPrefix(mimeType, "image/") {
            return errors.New("invalid file type for image")
        }
    case "video":
        if !strings.HasPrefix(mimeType, "video/") {
            return errors.New("invalid file type for video")
        }
    case "audio":
        if !strings.HasPrefix(mimeType, "audio/") {
            return errors.New("invalid file type for audio")
        }
    case "file":
        // Allow any type
    default:
        return errors.New("invalid message type")
    }
    
    return nil
}
```

### 3. Token Refresh Strategy

```go
// Always check expiry before using token
func shouldRefreshToken(expiry pgtype.Timestamptz) bool {
    if !expiry.Valid {
        return true
    }
    
    now := time.Now().UTC()
    return !expiry.Time.UTC().After(now)
}

// Refresh with buffer time (e.g., 1 day before expiry)
func shouldRefreshTokenWithBuffer(expiry pgtype.Timestamptz, buffer time.Duration) bool {
    if !expiry.Valid {
        return true
    }
    
    now := time.Now().UTC()
    threshold := now.Add(buffer)
    return !expiry.Time.UTC().After(threshold)
}
```

### 4. Error Handling

```go
// Always cleanup on upload failure
uploadResult, err := storage.CreateFile(bucketID, fileID, inputFile)
if err != nil {
    // Cleanup temp file
    os.Remove(tempFilePath)
    return nil, &ApiError{Code: 500, Message: "upload failed"}
}

// Cleanup Appwrite resources on database failure
message, err := queries.CreateMessageWithFile(ctx, params)
if err != nil {
    // Delete uploaded file
    storage.DeleteFile(bucketID, fileID)
    // Delete token
    tokens.Delete(tokenID)
    return nil, &ApiError{Code: 500, Message: "database error"}
}
```

### 5. File Cleanup

```go
// Always cleanup files after message deletion (follows business logic encapsulation)
func CleanupMessageFile(ctx context.Context, messageID uuid.UUID) error {
    message, err := queries.GetMessageByID(ctx, messageID)
    if err != nil {
        return err
    }
    
    // Delete all tokens for the file (business logic)
    if message.FileID != nil && *message.FileID != "" {
        ps.deleteAllFileTokens("chat_files", *message.FileID)
        if _, err := ps.Appwrite.Storage.DeleteFile("chat_files", *message.FileID); err != nil {
            log.Printf("[Appwrite] failed to delete file %s: %v", *message.FileID, err)
        }
    }
    
    // Delete thumbnail tokens and file (business logic)
    if message.ThumbnailFileID != nil && *message.ThumbnailFileID != "" {
        ps.deleteAllFileTokens("chat_files", *message.ThumbnailFileID)
        if _, err := ps.Appwrite.Storage.DeleteFile("chat_files", *message.ThumbnailFileID); err != nil {
            log.Printf("[Appwrite] failed to delete thumbnail %s: %v", *message.ThumbnailFileID, err)
        }
    }
    
    return nil
}

// Helper function to delete all tokens for a file (business logic)
func (ps *Service) deleteAllFileTokens(bucketID, fileID string) {
    if fileID == "" {
        return
    }

    tokenList, err := ps.Appwrite.Tokens.List(bucketID, fileID)
    if err != nil {
        log.Printf("[Appwrite] failed to list tokens for file %s: %v", fileID, err)
        return
    }

    for _, token := range tokenList.Tokens {
        if _, err := ps.Appwrite.Tokens.Delete(token.Id); err != nil {
            log.Printf("[Appwrite] failed to delete token %s for file %s: %v", token.Id, fileID, err)
        }
    }
}
```

### 6. Concurrent Access

```go
// Use database transactions for atomic operations
tx, err := db.Begin()
if err != nil {
    return err
}
defer tx.Rollback()

// Update token
err = queries.UpdateMessageFileToken(ctx, params)
if err != nil {
    return err
}

// Commit transaction
return tx.Commit()
```

## Token Management Safety (CRITICAL)

### Token Lifecycle Requirements
- **Always delete existing tokens before creating new ones**
- **Implement proper cleanup on database operation failures**
- **Follow token → file deletion sequence for cleanup**
- **Never leave orphaned tokens in Appwrite storage**

### Safe Token Refresh Pattern

```go
// ✅ SAFE PATTERN - Delete old tokens first
func (ps *Service) refreshToken(fileID string) (*string, error) {
    // 1. Delete existing tokens
    tokenList, err := ps.Appwrite.Tokens.List(bucketID, fileID)
    if err != nil {
        return nil, err
    }
    
    for _, token := range tokenList.Tokens {
        if _, err := ps.Appwrite.Tokens.Delete(token.Id); err != nil {
            log.Printf("Failed to delete old token %s: %v", token.Id, err)
        }
    }
    
    // 2. Create new token
    newToken, err := ps.Appwrite.Tokens.CreateFileToken(bucketID, fileID, ...)
    if err != nil {
        return nil, err
    }
    
    // 3. Update database
    err = ps.Queries.UpdateFileToken(ctx, params)
    if err != nil {
        // 4. Cleanup on failure - delete newly created token
        ps.Appwrite.Tokens.Delete(newToken.Id)
        return nil, err
    }
    
    return &newToken.Secret, nil
}
```

### Safe Cleanup Pattern

```go
// ✅ SAFE PATTERN - Token → File deletion sequence
func (ps *Service) cleanupFile(bucketID, fileID string) error {
    // 1. Delete all tokens first
    tokenList, err := ps.Appwrite.Tokens.List(bucketID, fileID)
    if err != nil {
        return err
    }
    
    for _, token := range tokenList.Tokens {
        if _, err := ps.Appwrite.Tokens.Delete(token.Id); err != nil {
            log.Printf("Failed to delete token %s: %v", token.Id, err)
        }
    }
    
    // 2. Then delete the file
    _, err = ps.Appwrite.Storage.DeleteFile(bucketID, fileID)
    if err != nil {
        return err
    }
    
    return nil
}
```

### Error Handling Requirements

```go
// ✅ SAFE PATTERN - Proper error handling and cleanup
result, err := ps.Appwrite.Storage.CreateFile(bucketID, fileID, inputFile)
if err != nil {
    return nil, &model.ApiError{
        Code:    http.StatusInternalServerError,
        Message: "Failed to upload file: " + err.Error(),
        Type:    "internal_server_error",
    }
}

// Create token after successful upload
token, err := ps.Appwrite.Tokens.CreateFileToken(bucketID, fileID, ...)
if err != nil {
    // Cleanup uploaded file on token creation failure
    ps.Appwrite.Storage.DeleteFile(bucketID, fileID)
    return nil, &model.ApiError{
        Code:    http.StatusInternalServerError,
        Message: "Failed to create token: " + err.Error(),
        Type:    "internal_server_error",
    }
}
```

---

## Troubleshooting

### Common Issues

#### 1. "Token not found" Error

**Symptom**: Download fails with "token not found"

**Causes**:
- Token was deleted
- Token expired and not refreshed
- Wrong token ID used

**Solution**:
```go
// Always use token SECRET in URLs, not ID
downloadURL := fmt.Sprintf(
    "%s/storage/buckets/%s/files/%s/download?project=%s&token=%s",
    endpoint, bucketID, fileID, projectID, 
    tokenSecret,  // Use SECRET, not ID
)
```

#### 2. "File not found" Error

**Symptom**: Download fails with "file not found"

**Causes**:
- File was deleted
- Wrong bucket ID
- Wrong file ID

**Solution**:
```go
// Verify file exists before building URL
fileMetadata, err := storage.GetFile(bucketID, fileID)
if err != nil {
    return error("file not found")
}
```

#### 3. "Invalid token" Error

**Symptom**: Download fails with "invalid token"

**Causes**:
- Token expired
- Token belongs to different file
- Token belongs to different bucket

**Solution**:
```go
// Always refresh expired tokens
if shouldRefreshToken(message.FileTokenExpiry) {
    newToken, err := tokens.CreateFileToken(bucketID, fileID, ...)
    // Update database with new token
}
```

#### 4. "Permission denied" Error

**Symptom**: Upload/download fails with permission error

**Causes**:
- API key lacks permissions
- Bucket permissions misconfigured
- User not authenticated

**Solution**:
```go
// Verify API key has correct permissions
// Check bucket settings in Appwrite console
// Ensure user is authenticated before file operations
```

#### 5. File Upload Timeout

**Symptom**: Upload fails after long wait

**Causes**:
- File too large
- Slow network
- Server timeout

**Solution**:
```go
// Set appropriate timeout
client := &http.Client{
    Timeout: 5 * time.Minute,
}

// Implement chunked upload for large files (future enhancement)
```

### Debugging Tips

#### 1. Enable Logging

```go
// Log all Appwrite operations
log.Printf("[Appwrite] Uploading file: %s to bucket: %s", fileID, bucketID)
uploadResult, err := storage.CreateFile(bucketID, fileID, inputFile)
if err != nil {
    log.Printf("[Appwrite] Upload failed: %v", err)
    return err
}
log.Printf("[Appwrite] Upload success: %s", uploadResult.Id)
```

#### 2. Verify Token Structure

```go
// Print token details for debugging
log.Printf("Token ID: %s", token.Id)
log.Printf("Token Secret: %s", token.Secret)
log.Printf("Token Expiry: %s", token.Expire)
```

#### 3. Test URL Manually

```bash
# Test download URL with curl
curl -v "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/{fileId}/download?project={projectId}&token={secret}"

# Check response headers
# Verify file downloads correctly
```

#### 4. Check Database State

```sql
-- Verify file metadata
SELECT id, file_id, file_token_id, file_token_secret, file_token_expiry
FROM messages
WHERE id = 'message-uuid';

-- Check for expired tokens
SELECT COUNT(*) 
FROM messages
WHERE file_token_expiry < now()
  AND file_id IS NOT NULL;
```

---

## Performance Optimization

### 1. Token Caching

```go
// Cache tokens in memory to avoid database queries
type TokenCache struct {
    tokens map[string]CachedToken
    mu     sync.RWMutex
}

type CachedToken struct {
    Secret string
    Expiry time.Time
}

func (tc *TokenCache) Get(fileID string) (string, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()
    
    token, exists := tc.tokens[fileID]
    if !exists || time.Now().After(token.Expiry) {
        return "", false
    }
    
    return token.Secret, true
}
```

### 2. Batch Token Refresh

```go
// Refresh multiple tokens in one operation
func RefreshExpiredTokens(ctx context.Context, limit int) error {
    messages, err := queries.GetMessagesWithExpiredFileTokens(ctx, limit)
    if err != nil {
        return err
    }
    
    for _, msg := range messages {
        // Refresh token
        newToken, err := tokens.CreateFileToken(bucketID, *msg.FileID, ...)
        if err != nil {
            log.Printf("Failed to refresh token for message %s: %v", msg.ID, err)
            continue
        }
        
        // Update database
        queries.UpdateMessageFileToken(ctx, ...)
    }
    
    return nil
}
```

### 3. CDN Integration

```go
// Use CDN for frequently accessed files
func BuildCDNURL(fileID string, token string) string {
    // Appwrite automatically uses CDN
    return fmt.Sprintf(
        "https://fra.cloud.appwrite.io/v1/storage/buckets/chat_files/files/%s/download?project=%s&token=%s",
        fileID, projectID, token,
    )
}
```

---

## Security Considerations

### 1. Token Security

- ✅ Never expose token IDs in public APIs
- ✅ Always use HTTPS for file URLs
- ✅ Set appropriate token expiry (1 year default)
- ✅ Delete tokens when files are deleted
- ✅ Verify user permissions before generating URLs

### 2. File Validation

- ✅ Validate file size before upload
- ✅ Validate MIME type
- ✅ Scan for malware (future enhancement)
- ✅ Check file extensions
- ✅ Limit file types per message type

### 3. Access Control

- ✅ Verify user is chat participant before file access
- ✅ Check messaging eligibility before upload
- ✅ Use private buckets (no public access)
- ✅ Implement rate limiting for uploads
- ✅ Log all file operations for audit

---

## Future Enhancements

### 1. Thumbnail Generation

```go
// Generate thumbnail for images/videos
func GenerateThumbnail(fileID string, mimeType string) (string, error) {
    if !strings.HasPrefix(mimeType, "image/") && 
       !strings.HasPrefix(mimeType, "video/") {
        return "", nil
    }
    
    // Download original file
    // Generate thumbnail (resize to 200x200)
    // Upload thumbnail to Appwrite
    // Return thumbnail file ID
}
```

### 2. File Compression

```go
// Compress large files before upload
func CompressFile(filePath string, mimeType string) (string, error) {
    if strings.HasPrefix(mimeType, "image/") {
        // Compress image (JPEG quality 85)
    } else if strings.HasPrefix(mimeType, "video/") {
        // Compress video (H.264)
    }
    
    return compressedPath, nil
}
```

### 3. Progress Tracking

```go
// Track upload progress
type UploadProgress struct {
    BytesUploaded int64
    TotalBytes    int64
    Percentage    float64
}

func UploadWithProgress(fileID string, reader io.Reader, size int64, 
                        progressChan chan<- UploadProgress) error {
    // Implement chunked upload with progress reporting
}
```

### 4. Background Jobs

```go
// Token refresh job
func StartTokenRefreshJob(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            RefreshExpiredTokens(context.Background(), 100)
        }
    }()
}

// File cleanup job
func StartFileCleanupJob(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            CleanupExpiredFiles(context.Background())
        }
    }()
}
```

---

## References

### Official Documentation
- [Appwrite Storage API](https://appwrite.io/docs/storage)
- [Appwrite Tokens API](https://appwrite.io/docs/tokens)
- [Appwrite Go SDK](https://github.com/appwrite/sdk-for-go)

### Internal Documentation
- `PHASE_6_FILE_MESSAGING_PLAN.md` - File messaging design
- `PHASE_6_FILE_MESSAGING_COMPLETE.md` - Implementation summary
- `PHASE_6_APPWRITE_FLOW_UPDATES.md` - URL pattern updates

### Test Files
- `@helper_cb_backend/go/testGo/main.go` - Appwrite flow reference

---

**Last Updated**: February 5, 2026  
**Maintained By**: ChatBasket Backend Team  
**Version**: 1.0.0
