DELETE FROM mail_outbox
WHERE template IN (
    'CUSTOMER_LOGIN_OTP',
    'CUSTOMER_PASSWORD_RESET_OTP'
);

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

DELETE FROM auth_refresh_tokens
WHERE session_id IN (
    SELECT id FROM auth_sessions WHERE scope = 'CUSTOMER'
);

DELETE FROM auth_sessions
WHERE scope = 'CUSTOMER';

DROP INDEX IF EXISTS ix_auth_sessions_customer_active;

ALTER TABLE auth_sessions
    DROP CONSTRAINT chk_auth_sessions_context,
    DROP CONSTRAINT fk_auth_sessions_customer,
    DROP COLUMN customer_id,
    ADD CONSTRAINT chk_auth_sessions_context CHECK (
        (scope = 'PLATFORM' AND cpo_id IS NULL AND role IS NULL)
        OR
        (
            scope = 'CPO'
            AND cpo_id IS NOT NULL
            AND role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER')
        )
    );

DROP INDEX IF EXISTS uq_customers_cpo_id_user_identity;

DELETE FROM auth_challenges
WHERE purpose IN (
    'CUSTOMER_LOGIN_2FA',
    'CUSTOMER_PASSWORD_RESET'
);

ALTER TABLE auth_challenges
    DROP CONSTRAINT chk_auth_challenges_purpose,
    DROP CONSTRAINT chk_auth_challenges_scope,
    ADD CONSTRAINT chk_auth_challenges_purpose
        CHECK (purpose IN ('LOGIN_2FA', 'PASSWORD_RESET')),
    ADD CONSTRAINT chk_auth_challenges_scope CHECK (
        (purpose = 'PASSWORD_RESET' AND scope IS NULL AND cpo_id IS NULL)
        OR
        (purpose = 'LOGIN_2FA' AND (
            (scope = 'PLATFORM' AND cpo_id IS NULL)
            OR
            (scope = 'CPO' AND cpo_id IS NOT NULL)
        ))
    );
