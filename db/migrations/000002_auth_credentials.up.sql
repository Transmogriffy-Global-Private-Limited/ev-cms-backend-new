ALTER TABLE users
    ADD COLUMN mfa_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN failed_login_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN locked_until timestamptz,
    ADD COLUMN last_login_at timestamptz,
    ADD CONSTRAINT chk_users_failed_login_attempts
        CHECK (failed_login_attempts >= 0);

CREATE TABLE auth_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    purpose varchar(30) NOT NULL,
    scope varchar(20),
    cpo_id uuid
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    code_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    invalidated_at timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    resend_available_at timestamptz NOT NULL,
    request_ip inet,
    user_agent varchar(512) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_auth_challenges_purpose
        CHECK (purpose IN ('LOGIN_2FA', 'PASSWORD_RESET')),
    CONSTRAINT chk_auth_challenges_scope
        CHECK (
            (purpose = 'PASSWORD_RESET' AND scope IS NULL AND cpo_id IS NULL)
            OR
            (purpose = 'LOGIN_2FA' AND (
                (scope = 'PLATFORM' AND cpo_id IS NULL)
                OR
                (scope = 'CPO' AND cpo_id IS NOT NULL)
            ))
        ),
    CONSTRAINT chk_auth_challenges_attempts
        CHECK (attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts),
    CONSTRAINT chk_auth_challenges_expiry
        CHECK (expires_at > created_at),
    CONSTRAINT chk_auth_challenges_terminal
        CHECK (consumed_at IS NULL OR invalidated_at IS NULL)
);

CREATE INDEX ix_auth_challenges_user_purpose_created
    ON auth_challenges (user_id, purpose, created_at DESC);
CREATE INDEX ix_auth_challenges_expiry
    ON auth_challenges (expires_at)
    WHERE consumed_at IS NULL AND invalidated_at IS NULL;

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    scope varchar(20) NOT NULL,
    cpo_id uuid
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    role varchar(20),
    token_version integer NOT NULL DEFAULT 1,
    ip_address inet,
    user_agent varchar(512) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoke_reason varchar(100),
    CONSTRAINT chk_auth_sessions_context CHECK (
        (scope = 'PLATFORM' AND cpo_id IS NULL AND role IS NULL)
        OR
        (
            scope = 'CPO'
            AND cpo_id IS NOT NULL
            AND role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER')
        )
    ),
    CONSTRAINT chk_auth_sessions_token_version CHECK (token_version > 0),
    CONSTRAINT chk_auth_sessions_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_auth_sessions_revocation
        CHECK (
            (revoked_at IS NULL AND revoke_reason IS NULL)
            OR
            (revoked_at IS NOT NULL AND revoke_reason IS NOT NULL)
        )
);

CREATE INDEX ix_auth_sessions_user_active
    ON auth_sessions (user_id, created_at DESC)
    WHERE revoked_at IS NULL;
CREATE INDEX ix_auth_sessions_cpo_active
    ON auth_sessions (cpo_id, created_at DESC)
    WHERE cpo_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE auth_refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL
        REFERENCES auth_sessions(id) ON UPDATE CASCADE ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    replacement_id uuid
        REFERENCES auth_refresh_tokens(id) ON UPDATE CASCADE ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_auth_refresh_token_hash
        CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_auth_refresh_token_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_auth_refresh_token_state
        CHECK (used_at IS NULL OR revoked_at IS NULL)
);

CREATE INDEX ix_auth_refresh_tokens_session_created
    ON auth_refresh_tokens (session_id, created_at DESC);
CREATE INDEX ix_auth_refresh_tokens_expiry
    ON auth_refresh_tokens (expires_at)
    WHERE used_at IS NULL AND revoked_at IS NULL;

CREATE TABLE mail_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    to_email varchar(320) NOT NULL,
    template varchar(50) NOT NULL,
    payload_ciphertext bytea NOT NULL,
    encryption_key_id varchar(50) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 8,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    last_error varchar(500),
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_mail_outbox_email CHECK (btrim(to_email) <> ''),
    CONSTRAINT chk_mail_outbox_template
        CHECK (template IN ('LOGIN_OTP', 'PASSWORD_RESET_OTP')),
    CONSTRAINT chk_mail_outbox_status
        CHECK (status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED')),
    CONSTRAINT chk_mail_outbox_attempts
        CHECK (attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts),
    CONSTRAINT chk_mail_outbox_terminal CHECK (
        (status = 'SENT' AND sent_at IS NOT NULL)
        OR
        (status <> 'SENT' AND sent_at IS NULL)
    )
);

CREATE INDEX ix_mail_outbox_delivery
    ON mail_outbox (status, available_at, created_at)
    WHERE status IN ('PENDING', 'PROCESSING');

CREATE TABLE auth_rate_limits (
    scope_key char(64) NOT NULL,
    action varchar(50) NOT NULL,
    window_started_at timestamptz NOT NULL,
    attempt_count integer NOT NULL DEFAULT 0,
    blocked_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_key, action),
    CONSTRAINT chk_auth_rate_limits_scope_key
        CHECK (scope_key ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_auth_rate_limits_action CHECK (btrim(action) <> ''),
    CONSTRAINT chk_auth_rate_limits_attempts CHECK (attempt_count >= 0)
);

CREATE INDEX ix_auth_rate_limits_blocked_until
    ON auth_rate_limits (blocked_until)
    WHERE blocked_until IS NOT NULL;
CREATE INDEX ix_auth_rate_limits_updated_at
    ON auth_rate_limits (updated_at);

CREATE TABLE cpo_integrations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    provider varchar(50) NOT NULL,
    credential_ciphertext bytea NOT NULL,
    encryption_key_id varchar(50) NOT NULL,
    display_hint varchar(100) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    updated_by_user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_integration UNIQUE (cpo_id, provider),
    CONSTRAINT chk_cpo_integrations_provider CHECK (provider = 'RAZORPAY'),
    CONSTRAINT chk_cpo_integrations_display_hint CHECK (btrim(display_hint) <> '')
);

CREATE INDEX ix_cpo_integrations_cpo_active
    ON cpo_integrations (cpo_id, is_active);
