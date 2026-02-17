ALTER TABLE messages
ADD COLUMN delivered_to_recipient_primary BOOLEAN DEFAULT FALSE;