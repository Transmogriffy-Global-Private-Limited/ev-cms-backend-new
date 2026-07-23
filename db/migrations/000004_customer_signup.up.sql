CREATE TABLE customer_signup_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    email varchar(320) NOT NULL,
    password_hash varchar(255) NOT NULL,
    full_name varchar(255) NOT NULL,
    phone varchar(32),
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
    CONSTRAINT chk_customer_signup_email CHECK (btrim(email) <> ''),
    CONSTRAINT chk_customer_signup_name CHECK (btrim(full_name) <> ''),
    CONSTRAINT chk_customer_signup_attempts CHECK (
        attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts
    ),
    CONSTRAINT chk_customer_signup_expiry CHECK (
        resend_available_at <= expires_at
    ),
    CONSTRAINT chk_customer_signup_terminal CHECK (
        consumed_at IS NULL OR invalidated_at IS NULL
    )
);

CREATE INDEX ix_customer_signup_cpo_email_created
    ON customer_signup_challenges (cpo_id, lower(btrim(email)), created_at DESC);
CREATE INDEX ix_customer_signup_expiry
    ON customer_signup_challenges (expires_at)
    WHERE consumed_at IS NULL AND invalidated_at IS NULL;

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP'
        )
    );
