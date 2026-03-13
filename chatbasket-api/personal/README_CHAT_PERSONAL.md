# Personal Chat Module

This module implements the **1-to-1 Ephemeral Chat** system for ChatBasket.

## Key Components

### 1. Database Layer (`internal/db/personal`)
*   **Migrations**: `008_personal_chat_system.up.sql` (Tables), `010_personal_chat_status_metadata.up.sql` (Read Status).
*   **Queries**: `queries/personal_chat.sql`.
*   **Generated Code**: `internal/db/personal/personal_chat.sql.go`.

### 2. Service Layer (`personalservice/chat_service.go`)
Handles business logic:
*   `SendMessage`: Checks eligibility -> Creates Chat (if needed) -> Inserts Message -> Updates Chat Metadata.
*   `GetMessages`: Fetches messages + calculates `last_message_status` (sent/read).
*   `MarkChatRead`: Updates `p*_last_read_at` timestamp in `chats` table.
*   `AcknowledgeDelivery`: Updates `delivered_to_recipient` (used for deletion eligibility).

### 3. Handler Layer (`personalhandler/chat_handler.go`)
*   Thin wrapper around service.
*   Validates UUIDs.
*   Maps Service Models to JSON Responses.

## 4. Synchronization Strategy
The system differentiates between **Delivery ACKs** (initiated from Inbox/Home or on Startup) and **Read ACKs** (initiated from within a Chat).

*   **Outside Chat / Boot-up (Delivery)**: When new messages arrive via WebSocket, or when the app hydrates offline messages on startup, the client automatically triggers a "Single ACK" time-based call to `AcknowledgeDelivery` for the *latest* message. The backend uses the timestamp of this message to horizontally update `delivered_to_recipient = TRUE` for it and *all older pending messages*, ensuring efficient bulk delivery state synchronization.
*   **Inside Chat (Read)**: When the user opens a chat, the client calls `MarkChatRead`. 
    *   **Implicit Delivery**: `MarkChatRead` internally performs a bulk delivery ACK for all messages in the chat. This ensures that even if individual delivery signals were missed, the state remains consistent.
    *   **Unread Count**: Resets the chat's unread counter to 0.

## 5. Architecture Notes
*   **Status Truth**: The `messages` table does **not** store "Read" status per message. It only stores "Delivered".
*   **Read Logic**: "Read" is a computed state based on `chat.p*_last_read_at` vs `message.created_at`.
*   **Deletion**: Deletion logic (Ephemerality) relies on `delivered_to_recipient=TRUE`.

For high-level architecture, see `docs/system-map.md`.
