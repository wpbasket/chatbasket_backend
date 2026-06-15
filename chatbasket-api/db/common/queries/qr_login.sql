-- name: CreateQRLoginRequest :one
INSERT INTO qr_login_requests (
    id, status, expires_at, created_at, updated_at
) VALUES (
    $1, 'PENDING', $2, NOW(), NOW()
) RETURNING id;

-- name: GetQRLoginRequest :one
SELECT id, auth_user_id, status, expires_at
FROM qr_login_requests
WHERE id = $1 AND status = 'PENDING' AND expires_at > NOW();

-- name: ApproveQRLogin :one
UPDATE qr_login_requests
SET status = 'APPROVED', auth_user_id = $2, updated_at = NOW()
WHERE id = $1 AND status = 'PENDING' AND expires_at > NOW()
RETURNING id;

-- name: ExchangeQRLogin :one
UPDATE qr_login_requests
SET status = 'EXCHANGED', updated_at = NOW()
WHERE id = $1 AND status = 'APPROVED' AND expires_at > NOW()
RETURNING auth_user_id;

-- name: CleanupExpiredQRLoginRequests :exec
DELETE FROM qr_login_requests
WHERE expires_at < NOW();

-- name: NotifyQREvent :exec
SELECT pg_notify('qr_login_events', $1::text);
