-- +migrate Down

-- Drop auth_rate_limiters
DROP INDEX IF EXISTS idx_auth_rate_limiters_active_state;
DROP TRIGGER IF EXISTS auth_rate_limiters_timestamps_trigger ON auth_rate_limiters;
DROP TABLE IF EXISTS auth_rate_limiters CASCADE;
