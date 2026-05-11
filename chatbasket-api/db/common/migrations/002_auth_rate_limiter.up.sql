-- +migrate Up

-- ======================================
-- Create auth_rate_limiters table
-- ======================================

-- Create table
CREATE TABLE IF NOT EXISTS auth_rate_limiters (
    auth_user_id UUID PRIMARY KEY REFERENCES auth_users(id) ON DELETE CASCADE,
    
    -- Send Limits
    otp_hourly_count INT NOT NULL DEFAULT 0,
    otp_daily_count INT NOT NULL DEFAULT 0,
    last_otp_send_at TIMESTAMPTZ,
    daily_reset_at TIMESTAMPTZ DEFAULT now(),
    
    -- Verify Limits (Brute Force Protection)
    otp_verify_errors INT NOT NULL DEFAULT 0,
    last_verify_attempt_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Drop existing trigger if present (safe idempotent)
DROP TRIGGER IF EXISTS auth_rate_limiters_timestamps_trigger ON auth_rate_limiters;

-- Attach timestamp trigger (expects set_timestamps() function already exists)
CREATE TRIGGER auth_rate_limiters_timestamps_trigger
BEFORE INSERT OR UPDATE ON auth_rate_limiters
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- ======================================
-- Indexes
-- ======================================

-- Partial index for users with active rate-limiting state (counters or errors > 0)
-- This is the most efficient index for monitoring or bulk-resetting active limiters,
-- as it excludes the vast majority of "clean" or inactive records.
CREATE INDEX IF NOT EXISTS idx_auth_rate_limiters_active_state
    ON auth_rate_limiters (auth_user_id)
    WHERE otp_hourly_count > 0 
       OR otp_daily_count > 0 
       OR otp_verify_errors > 0;
