-- +migrate Up

-- ======================================
-- Table: qr_login_requests
--        Short-lived rows for QR-based web login
-- ======================================
CREATE TABLE IF NOT EXISTS qr_login_requests (
    id                  UUID            PRIMARY KEY,  -- UUIDv7 shown in QR code
    auth_user_id        UUID            REFERENCES auth_users (id) ON DELETE CASCADE,  -- NULL while PENDING, set on approve
    signal_offer        TEXT,           -- Browser's SDP offer (JSON text)
    signal_answer       TEXT,           -- Mobile's SDP answer (JSON text)
    status              TEXT            NOT NULL DEFAULT 'PENDING',
    CONSTRAINT qr_login_requests_status_check CHECK (
        status IN ('PENDING', 'APPROVED', 'EXCHANGED')
    ),
    expires_at          TIMESTAMPTZ     NOT NULL,
    created_at          TIMESTAMPTZ     NOT NULL,
    updated_at          TIMESTAMPTZ     NOT NULL
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS qr_login_requests_timestamps_trigger ON qr_login_requests;

-- Attach auto timestamp trigger
CREATE TRIGGER qr_login_requests_timestamps_trigger
BEFORE INSERT OR UPDATE ON qr_login_requests
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- ======================================
-- End of qr_login_requests table section
-- ======================================
