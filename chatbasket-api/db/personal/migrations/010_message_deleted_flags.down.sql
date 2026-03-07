ALTER TABLE messages DROP COLUMN IF EXISTS deleted_by_sender;

ALTER TABLE messages DROP COLUMN IF EXISTS deleted_by_recipient;