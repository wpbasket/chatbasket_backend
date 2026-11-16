-- +migrate Up
-- ======================================
-- Table: message_history
--        Append-only tombstone of message IDs partitioned by range on created_at.
--        Enables two-tier deduplication: if a message row was purged from the relay
--        upon delivery/read/TTL, this table records that the ID was already processed,
--        allowing the server to reject duplicate re-inserts. Daily partitions (-1d..+7d)
--        and 33-day retention drops are managed by personal_chat partition maintenance.
-- ======================================
CREATE TABLE IF NOT EXISTS message_history (
    id          UUID         NOT NULL,                                  -- Client-generated message UUID
    created_at  TIMESTAMPTZ  NOT NULL,                                  -- Server message timestamp (partition key)
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- ======================================
-- End of message_history table section
-- ======================================
