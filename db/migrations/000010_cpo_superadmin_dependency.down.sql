DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM mail_outbox
        WHERE template = 'CPO_ONBOARDING_RESENT'
    ) THEN
        RAISE EXCEPTION
            'cannot roll back CPO Superadmin dependency while onboarding resend mail exists';
    END IF;
END
$$;

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
            'CUSTOMER_PASSWORD_RESET_OTP',
            'CPO_SUBSCRIPTION_CHANGED',
            'CPO_PLATFORM_INVOICE_ISSUED'
        )
    );

DROP INDEX IF EXISTS ix_mail_outbox_cpo_user_created;

ALTER TABLE mail_outbox
    DROP COLUMN IF EXISTS user_id,
    DROP COLUMN IF EXISTS cpo_id;

DROP INDEX IF EXISTS uq_cpo_memberships_primary_admin;

ALTER TABLE cpo_memberships
    DROP CONSTRAINT IF EXISTS chk_cpo_memberships_primary_admin_role,
    DROP COLUMN IF EXISTS is_primary_admin;

DROP INDEX IF EXISTS ix_cpos_status_created;

ALTER TABLE cpos
    DROP CONSTRAINT IF EXISTS chk_cpos_status_reason,
    DROP COLUMN IF EXISTS status_changed_by_user_id,
    DROP COLUMN IF EXISTS status_changed_at,
    DROP COLUMN IF EXISTS status_reason;
