-- Add deletion flags independent for sender and recipient
ALTER TABLE messages
ADD COLUMN deleted_by_sender BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE messages
ADD COLUMN deleted_by_recipient BOOLEAN NOT NULL DEFAULT FALSE;