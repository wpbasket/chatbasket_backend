-- +migrate Down

-- Drop message_history
DROP TABLE IF EXISTS message_history CASCADE;                           -- Also drops all attached partitions and PK constraint
