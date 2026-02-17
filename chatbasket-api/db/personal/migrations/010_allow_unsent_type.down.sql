-- +migrate Down
ALTER TABLE messages
DROP CONSTRAINT IF EXISTS messages_file_type_validation;

ALTER TABLE messages
DROP CONSTRAINT IF EXISTS messages_message_type_check;

ALTER TABLE messages
ADD CONSTRAINT messages_message_type_check CHECK (
    message_type IN (
        'text',
        'image',
        'video',
        'audio',
        'file'
    )
);

ALTER TABLE messages
ADD CONSTRAINT messages_file_type_validation CHECK (
    (
        message_type = 'text'
        AND file_id IS NULL
    )
    OR (
        message_type IN (
            'image',
            'video',
            'audio',
            'file'
        )
        AND file_id IS NOT NULL
    )
);