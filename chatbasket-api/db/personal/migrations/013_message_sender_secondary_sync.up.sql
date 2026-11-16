-- +migrate Up

-- Which sender session authored this message. Set only from the session middleware at insert.
ALTER TABLE messages
    ADD COLUMN sender_session_id UUID NOT NULL;
