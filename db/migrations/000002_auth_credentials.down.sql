DROP TABLE IF EXISTS cpo_integrations;
DROP TABLE IF EXISTS auth_rate_limits;
DROP TABLE IF EXISTS mail_outbox;
DROP TABLE IF EXISTS auth_refresh_tokens;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS auth_challenges;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_failed_login_attempts,
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_login_attempts,
    DROP COLUMN IF EXISTS password_changed_at,
    DROP COLUMN IF EXISTS mfa_enabled;
