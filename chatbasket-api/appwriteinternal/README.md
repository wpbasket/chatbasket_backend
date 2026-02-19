# appwriteinternal/

Internal wrapper for Appwrite Go SDK.

## Services

### `AppwriteService` (standard)
- **Timeout**: 30 Seconds.
- **Use for**: Auth, Database queries, Messaging, and Storage management (List, Delete, Token generation).
- **Client wiring**: Initialized in `routes/routes.go` and included in `GlobalService`.

### `AppwriteStorageService` (high-timeout)
- **Timeout**: 10 Minutes (600 Seconds).
- **Use for**: ONLY `Storage.CreateFile` (uploads).
- **Why**: Large file uploads (up to 100MB) can take several minutes on slower connections. Using a separate client prevents long-running uploads from being affected by the standard 30s timeout used for other logic.

## Usage in Services
Access these via the `GlobalService` struct:
```go
// For uploads
gs.AppwriteStorage.Storage.CreateFile(...)

// For everything else
gs.Appwrite.Database.ListDocuments(...)
gs.Appwrite.Storage.DeleteFile(...)
```
