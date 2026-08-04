DROP TABLE IF EXISTS platform_notifications;
DROP TABLE IF EXISTS platform_announcements;

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    DROP CONSTRAINT chk_mail_outbox_status,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP', 'PASSWORD_RESET_OTP', 'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED', 'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP', 'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP', 'CPO_ONBOARDING_RESENT',
            'CPO_SUBSCRIPTION_CHANGED', 'CPO_PLATFORM_INVOICE_ISSUED'
        )
    ),
    ADD CONSTRAINT chk_mail_outbox_status CHECK (
        status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED')
    );

ALTER TABLE platform_admins
    DROP CONSTRAINT chk_platform_admin_status_reason,
    DROP COLUMN updated_at,
    DROP COLUMN status_changed_by_user_id,
    DROP COLUMN status_changed_at,
    DROP COLUMN status_reason,
    DROP COLUMN is_active;
