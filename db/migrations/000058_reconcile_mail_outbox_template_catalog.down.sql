-- Rolling back would restore the preceding 000014 catalogue. Refuse to lose
-- enforcement for rows that only the current catalogue can represent.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM mail_outbox
        WHERE template IN (
            'CPO_STAFF_NEW_IDENTITY',
            'CPO_STAFF_EXISTING_IDENTITY',
            'CPO_STAFF_ROLE_CHANGED',
            'CPO_STAFF_SUSPENDED',
            'CPO_STAFF_REACTIVATED',
            'CPO_STAFF_REVOKED',
            'CPO_SUBSCRIPTION_EXPIRY_WARNING',
            'CPO_SUBSCRIPTION_EXPIRED',
            'CPO_SUPPORT_TICKET_CREATED',
            'CPO_SUPPORT_TICKET_PLATFORM_REPLY',
            'CPO_SUPPORT_TICKET_RESOLVED',
            'CPO_SUPPORT_TICKET_CLOSED',
            'CPO_SUPPORT_TICKET_REOPENED'
        )
    ) THEN
        RAISE EXCEPTION 'cannot roll back mail outbox template catalogue while current semantic mail rows exist';
    END IF;
END $$;

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS chk_mail_outbox_template,
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
            'CPO_ONBOARDING_RESENT',
            'CPO_SUBSCRIPTION_CHANGED',
            'CPO_PLATFORM_INVOICE_ISSUED',
            'PLATFORM_ADMIN_INVITE',
            'PLATFORM_ADMIN_GRANTED'
        )
    );
