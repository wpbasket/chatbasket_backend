-- +migrate Up

-- ======================================
-- Table: chats
--        Stores 1v1 chat metadata (not messages themselves)
-- ======================================
CREATE TABLE IF NOT EXISTS chats (
    id                                  UUID            PRIMARY KEY,  -- Go-generated UUID
    participant_1_id                    UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    participant_2_id                    UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,

-- Unread Counts & Read Status
p1_unread_count INTEGER NOT NULL DEFAULT 0,
p2_unread_count INTEGER NOT NULL DEFAULT 0,
p1_last_read_at TIMESTAMPTZ,
p2_last_read_at TIMESTAMPTZ,

-- Last Message Preview (Persistent even if message is deleted)
last_message_content                TEXT,
    last_message_created_at             TIMESTAMPTZ,
    last_message_type                   TEXT,
    last_message_sender_id              UUID,

    created_at                          TIMESTAMPTZ,
    updated_at                          TIMESTAMPTZ,
    CONSTRAINT chats_unique_pair UNIQUE(participant_1_id, participant_2_id),
    CONSTRAINT chats_no_self_chat CHECK (participant_1_id != participant_2_id),
    CONSTRAINT chats_ordered_pair CHECK (participant_1_id < participant_2_id)
);

-- Add comments for developer clarity
COMMENT ON COLUMN chats.p1_unread_count IS 'Unread count for participant_1_id';

COMMENT ON COLUMN chats.p2_unread_count IS 'Unread count for participant_2_id';

COMMENT ON COLUMN chats.last_message_content IS 'Content of the last message (persisted for preview)';

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS chats_timestamps_trigger ON chats;

-- Attach auto timestamp trigger
CREATE TRIGGER chats_timestamps_trigger
BEFORE INSERT OR UPDATE ON chats
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit index for participant lookups
CREATE INDEX IF NOT EXISTS idx_chats_participant_1 ON chats (
    participant_1_id,
    created_at DESC
);

CREATE INDEX IF NOT EXISTS idx_chats_participant_2 ON chats (
    participant_2_id,
    created_at DESC
);
-- ======================================
-- End of chats table section
-- ======================================

-- ======================================
-- Table: messages
--        Temporary message relay storage (ephemeral)
-- ======================================
CREATE TABLE IF NOT EXISTS messages (
    id                                  UUID            PRIMARY KEY,  -- Go-generated UUID
    chat_id                             UUID            NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id                           UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id                        UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,

-- Message content (encrypted at device level in future E2EE phase)
content TEXT NOT NULL CHECK (length(content) <= 5000),
message_type TEXT NOT NULL DEFAULT 'text' CHECK (
    message_type IN (
        'text',
        'image',
        'video',
        'audio',
        'file'
    )
),

-- File Attachment Fields
file_id TEXT,
file_name TEXT,
file_size BIGINT,
file_mime_type TEXT,
file_token_id TEXT,
file_token_secret TEXT,
file_token_expiry TIMESTAMPTZ,
thumbnail_file_id TEXT,
thumbnail_token_id TEXT,
thumbnail_token_secret TEXT,

-- Delivery tracking for primary-device-centric relay
delivered_to_recipient BOOLEAN NOT NULL DEFAULT FALSE,
synced_to_sender_primary BOOLEAN NOT NULL DEFAULT FALSE,

-- Retry and TTL management
delivery_attempts INTEGER NOT NULL DEFAULT 0,
expires_at TIMESTAMPTZ NOT NULL, -- 30-day TTL default
created_at TIMESTAMPTZ,
updated_at TIMESTAMPTZ,
CONSTRAINT messages_valid_participants CHECK (sender_id != recipient_id),

-- File Attachment Constraints
CONSTRAINT messages_file_size_limit
        CHECK (file_size IS NULL OR file_size <= 104857600), -- 100MB limit
    CONSTRAINT messages_file_type_validation
        CHECK (
            (message_type = 'text' AND file_id IS NULL) OR
            (message_type IN ('image', 'video', 'audio', 'file') AND file_id IS NOT NULL)
        )
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS messages_timestamps_trigger ON messages;

-- Attach auto timestamp trigger
CREATE TRIGGER messages_timestamps_trigger
BEFORE INSERT OR UPDATE ON messages
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index for pending delivery queries (critical for relay performance)
CREATE INDEX IF NOT EXISTS idx_messages_pending_delivery ON messages (
    recipient_id,
    delivered_to_recipient,
    expires_at
)
WHERE
    delivered_to_recipient = FALSE;

-- Index for sender primary sync queue
CREATE INDEX IF NOT EXISTS idx_messages_pending_sender_sync ON messages (
    sender_id,
    synced_to_sender_primary,
    expires_at
)
WHERE
    synced_to_sender_primary = FALSE;

-- Index for TTL cleanup job
CREATE INDEX IF NOT EXISTS idx_messages_expired ON messages (expires_at)
WHERE
    expires_at IS NOT NULL;

-- Index for chat history retrieval
CREATE INDEX IF NOT EXISTS idx_messages_chat_history ON messages (chat_id, created_at DESC);

-- Index for file cleanup
CREATE INDEX IF NOT EXISTS idx_messages_file_cleanup ON messages (file_id)
WHERE
    file_id IS NOT NULL;

-- Index for expired file tokens
CREATE INDEX IF NOT EXISTS idx_messages_expired_file_tokens ON messages (file_token_expiry)
WHERE
    file_id IS NOT NULL
    AND file_token_expiry IS NOT NULL;

-- ======================================
-- End of messages table section
-- ======================================

-- ======================================
-- Table: message_delivery_log
--        Audit trail for delivery failures and retries
-- ======================================
CREATE TABLE IF NOT EXISTS message_delivery_log (
    id UUID PRIMARY KEY, -- Go-generated UUID
    message_id UUID NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN (
            'pending',
            'delivered',
            'failed',
            'expired'
        )
    ),
    error_reason TEXT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS message_delivery_log_timestamps_trigger ON message_delivery_log;

-- Attach auto timestamp trigger
CREATE TRIGGER message_delivery_log_timestamps_trigger
BEFORE INSERT OR UPDATE ON message_delivery_log
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit index for message delivery history
CREATE INDEX IF NOT EXISTS idx_delivery_log_message ON message_delivery_log (message_id, attempted_at DESC);
-- ======================================
-- End of message_delivery_log table section
-- ======================================

-- ======================================
-- End of migration: Phase 6 Chat System (Merged)
-- ======================================