# services/

Business logic layer. Services orchestrate db queries (sqlc) and utils (OTP, hashing, sessions). Handlers call services; services should not depend on Echo. Return typed `model` structs and `*model.ApiError`.

### Global Service & Appwrite integration
The `GlobalService` holds references to shared clients. For Appwrite, we maintain two distinct services in the `appwriteinternal` package:
- `Appwrite`: Standard client with a **30-second** timeout for Auth and Database operations.
- `AppwriteStorage`: Dedicated client with a **10-minute** timeout, used exclusively for large file uploads (`CreateFile`). 

Other storage operations like `DeleteFile` or `ListFiles` should use the standard `Appwrite` service to ensure fast failures.
### Chat Synchronization Strategy
The chat system distinguishes between **Delivery ACKs** (Recipient Confirmed) and **Read ACKs** (User Opened).
*   **Outside Chat**: Small, frequent delivery signals marked as `acknowledged_by: 'recipient'`.
*   **Inside Chat**: Bulk read signal via `MarkChatRead`, which implicitly updates all delivery flags for the chat to ensure consistency.

Other storage operations like `DeleteFile` or `ListFiles` should use the standard `Appwrite` service to ensure fast failures.
