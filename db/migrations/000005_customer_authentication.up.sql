ALTER TABLE auth_challenges
    DROP CONSTRAINT chk_auth_challenges_purpose,
    DROP CONSTRAINT chk_auth_challenges_scope,
    ADD CONSTRAINT chk_auth_challenges_purpose CHECK (
        purpose IN (
            'LOGIN_2FA',
            'PASSWORD_RESET',
            'CUSTOMER_LOGIN_2FA',
            'CUSTOMER_PASSWORD_RESET'
        )
    ),
    ADD CONSTRAINT chk_auth_challenges_scope CHECK (
        (purpose = 'PASSWORD_RESET' AND scope IS NULL AND cpo_id IS NULL)
        OR
        (purpose = 'LOGIN_2FA' AND (
            (scope = 'PLATFORM' AND cpo_id IS NULL)
            OR
            (scope = 'CPO' AND cpo_id IS NOT NULL)
        ))
        OR
        (
            purpose IN ('CUSTOMER_LOGIN_2FA', 'CUSTOMER_PASSWORD_RESET')
            AND scope = 'CUSTOMER'
            AND cpo_id IS NOT NULL
        )
    );

CREATE UNIQUE INDEX uq_customers_cpo_id_user_identity
    ON customers (cpo_id, id, user_id);

ALTER TABLE auth_sessions
    ADD COLUMN customer_id uuid,
    DROP CONSTRAINT chk_auth_sessions_context,
    ADD CONSTRAINT fk_auth_sessions_customer
        FOREIGN KEY (cpo_id, customer_id, user_id)
        REFERENCES customers(cpo_id, id, user_id)
        ON UPDATE CASCADE ON DELETE CASCADE,
    ADD CONSTRAINT chk_auth_sessions_context CHECK (
        (
            scope = 'PLATFORM'
            AND cpo_id IS NULL
            AND role IS NULL
            AND customer_id IS NULL
        )
        OR
        (
            scope = 'CPO'
            AND cpo_id IS NOT NULL
            AND role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER')
            AND customer_id IS NULL
        )
        OR
        (
            scope = 'CUSTOMER'
            AND cpo_id IS NOT NULL
            AND role IS NULL
            AND customer_id IS NOT NULL
        )
    );

CREATE INDEX ix_auth_sessions_customer_active
    ON auth_sessions (cpo_id, customer_id, created_at DESC)
    WHERE scope = 'CUSTOMER' AND revoked_at IS NULL;

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP',
            'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP'
        )
    );
