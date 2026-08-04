-- Customer credentials are CPO-local. This decision was made before any
-- customers existed; do not silently reinterpret historical credentials.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customers) THEN
        RAISE EXCEPTION '000020 requires an empty customers table; migrate customer identities explicitly before applying it';
    END IF;
END $$;

DROP INDEX IF EXISTS ix_customers_user;
DROP INDEX IF EXISTS ix_auth_sessions_customer_active;
ALTER TABLE auth_sessions DROP CONSTRAINT IF EXISTS fk_auth_sessions_customer;
DROP INDEX IF EXISTS uq_customers_cpo_id_user_identity;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS uq_cpo_customer;
ALTER TABLE customers DROP COLUMN IF EXISTS user_id;
ALTER TABLE customers
    ADD COLUMN email varchar(320) NOT NULL DEFAULT '',
    ADD COLUMN password_hash varchar(255) NOT NULL DEFAULT '',
    ADD COLUMN full_name varchar(255) NOT NULL DEFAULT '',
    ADD COLUMN phone varchar(32),
    ADD COLUMN is_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN failed_login_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN locked_until timestamptz,
    ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_login_at timestamptz;
ALTER TABLE customers ALTER COLUMN email DROP DEFAULT;
ALTER TABLE customers ALTER COLUMN password_hash DROP DEFAULT;
ALTER TABLE customers ALTER COLUMN full_name DROP DEFAULT;
ALTER TABLE customers
    ADD CONSTRAINT chk_customers_email_nonblank CHECK (btrim(email) <> ''),
    ADD CONSTRAINT chk_customers_password_hash_nonblank CHECK (btrim(password_hash) <> ''),
    ADD CONSTRAINT chk_customers_full_name_nonblank CHECK (btrim(full_name) <> ''),
    ADD CONSTRAINT chk_customers_failed_login_attempts CHECK (failed_login_attempts >= 0);
CREATE UNIQUE INDEX uq_cpo_customer_email ON customers (cpo_id, lower(btrim(email)));
CREATE UNIQUE INDEX uq_customers_cpo_id_identity ON customers (cpo_id, id);

DELETE FROM auth_refresh_tokens
WHERE session_id IN (SELECT id FROM auth_sessions WHERE scope = 'CUSTOMER');
DELETE FROM auth_sessions WHERE scope = 'CUSTOMER';
DELETE FROM auth_challenges
WHERE purpose IN ('CUSTOMER_LOGIN_2FA', 'CUSTOMER_PASSWORD_RESET');
ALTER TABLE auth_sessions
    DROP CONSTRAINT chk_auth_sessions_context,
    ADD CONSTRAINT chk_auth_sessions_context CHECK (
        (scope = 'PLATFORM' AND cpo_id IS NULL AND role IS NULL AND customer_id IS NULL)
        OR
        (scope = 'CPO' AND cpo_id IS NOT NULL AND role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER') AND customer_id IS NULL)
    );
ALTER TABLE auth_challenges
    DROP CONSTRAINT chk_auth_challenges_purpose,
    DROP CONSTRAINT chk_auth_challenges_scope,
    ADD CONSTRAINT chk_auth_challenges_purpose CHECK (purpose IN ('LOGIN_2FA', 'PASSWORD_RESET')),
    ADD CONSTRAINT chk_auth_challenges_scope CHECK (
        (purpose = 'PASSWORD_RESET' AND scope IS NULL AND cpo_id IS NULL)
        OR
        (purpose = 'LOGIN_2FA' AND (
            (scope = 'PLATFORM' AND cpo_id IS NULL)
            OR (scope = 'CPO' AND cpo_id IS NOT NULL)
        ))
    );

CREATE TABLE customer_auth_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    customer_id uuid NOT NULL,
    purpose varchar(30) NOT NULL CHECK (purpose IN ('CUSTOMER_LOGIN_2FA', 'CUSTOMER_PASSWORD_RESET')),
    code_hash bytea NOT NULL, expires_at timestamptz NOT NULL, consumed_at timestamptz,
    invalidated_at timestamptz, attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL, resend_available_at timestamptz NOT NULL,
    request_ip inet, user_agent varchar(512) NOT NULL DEFAULT '', created_at timestamptz NOT NULL,
    CONSTRAINT fk_customer_auth_challenge_account FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT chk_customer_auth_challenge_attempts CHECK (attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts)
);
CREATE INDEX ix_customer_auth_challenges_customer ON customer_auth_challenges (cpo_id, customer_id, purpose);
CREATE TABLE customer_auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    customer_id uuid NOT NULL,
    token_version integer NOT NULL DEFAULT 1, ip_address inet, user_agent varchar(512) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
    revoked_at timestamptz, revoke_reason varchar(100),
    CONSTRAINT fk_customer_auth_session_account FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT chk_customer_auth_session_expiry CHECK (expires_at > created_at)
);
CREATE INDEX ix_customer_auth_sessions_customer ON customer_auth_sessions (cpo_id, customer_id, expires_at);
CREATE TABLE customer_auth_refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES customer_auth_sessions(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    token_hash char(64) NOT NULL UNIQUE, expires_at timestamptz NOT NULL, used_at timestamptz,
    revoked_at timestamptz, replacement_id uuid, created_at timestamptz NOT NULL,
    CONSTRAINT fk_customer_refresh_replacement FOREIGN KEY (replacement_id)
        REFERENCES customer_auth_refresh_tokens(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_customer_refresh_expiry CHECK (expires_at > created_at)
);
CREATE INDEX ix_customer_auth_refresh_tokens_session ON customer_auth_refresh_tokens (session_id);
