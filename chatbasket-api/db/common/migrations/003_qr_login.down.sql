-- +migrate Down

-- Drop qr_login_requests
DROP TRIGGER IF EXISTS qr_login_requests_timestamps_trigger ON qr_login_requests;
DROP TABLE IF EXISTS qr_login_requests CASCADE;
