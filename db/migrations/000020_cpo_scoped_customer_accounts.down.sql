DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customers) THEN
        RAISE EXCEPTION '000020 down requires an empty customers table; CPO-local credentials cannot be reconstructed as global identities';
    END IF;
END $$;

DROP TABLE IF EXISTS customer_auth_refresh_tokens;
DROP TABLE IF EXISTS customer_auth_sessions;
DROP TABLE IF EXISTS customer_auth_challenges;
DROP INDEX IF EXISTS uq_customers_cpo_id_identity;
DROP INDEX IF EXISTS uq_cpo_customer_email;
ALTER TABLE customers
    ADD COLUMN user_id uuid NOT NULL,
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS password_changed_at,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_login_attempts,
    DROP COLUMN IF EXISTS is_verified,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS full_name,
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS email;
ALTER TABLE customers
    ADD CONSTRAINT customers_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT uq_cpo_customer UNIQUE (cpo_id, user_id);
CREATE INDEX ix_customers_user ON customers (user_id);
CREATE UNIQUE INDEX uq_customers_cpo_id_user_identity ON customers (cpo_id, id, user_id);

ALTER TABLE auth_challenges
    DROP CONSTRAINT chk_auth_challenges_purpose,
    DROP CONSTRAINT chk_auth_challenges_scope,
    ADD CONSTRAINT chk_auth_challenges_purpose CHECK (
        purpose IN ('LOGIN_2FA', 'PASSWORD_RESET', 'CUSTOMER_LOGIN_2FA', 'CUSTOMER_PASSWORD_RESET')
    ),
    ADD CONSTRAINT chk_auth_challenges_scope CHECK (
        (purpose = 'PASSWORD_RESET' AND scope IS NULL AND cpo_id IS NULL)
        OR (purpose = 'LOGIN_2FA' AND ((scope = 'PLATFORM' AND cpo_id IS NULL) OR (scope = 'CPO' AND cpo_id IS NOT NULL)))
        OR (purpose IN ('CUSTOMER_LOGIN_2FA', 'CUSTOMER_PASSWORD_RESET') AND scope = 'CUSTOMER' AND cpo_id IS NOT NULL)
    );
ALTER TABLE auth_sessions
    DROP CONSTRAINT chk_auth_sessions_context,
    ADD CONSTRAINT fk_auth_sessions_customer FOREIGN KEY (cpo_id, customer_id, user_id)
        REFERENCES customers(cpo_id, id, user_id) ON UPDATE CASCADE ON DELETE CASCADE,
    ADD CONSTRAINT chk_auth_sessions_context CHECK (
        (scope = 'PLATFORM' AND cpo_id IS NULL AND role IS NULL AND customer_id IS NULL)
        OR (scope = 'CPO' AND cpo_id IS NOT NULL AND role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER') AND customer_id IS NULL)
        OR (scope = 'CUSTOMER' AND cpo_id IS NOT NULL AND role IS NULL AND customer_id IS NOT NULL)
    );
CREATE INDEX ix_auth_sessions_customer_active ON auth_sessions (cpo_id, customer_id, created_at DESC)
WHERE scope = 'CUSTOMER' AND revoked_at IS NULL;
