-- +migrate Down

ALTER TABLE messages
    DROP COLUMN IF EXISTS sender_session_id;
