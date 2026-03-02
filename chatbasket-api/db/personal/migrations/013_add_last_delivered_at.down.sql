-- +migrate Down
ALTER TABLE chats
DROP COLUMN IF EXISTS p1_last_delivered_at,
DROP COLUMN IF EXISTS p2_last_delivered_at;