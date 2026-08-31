-- Keep the durable mail-outbox boundary aligned with the templates the
-- application can validate and render. NOT VALID preserves existing historical
-- rows that were accepted by the obsolete catalogue while enforcing this exact
-- catalogue for every new or updated row.
ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_SIGNUP_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER',
            'PLATFORM_ADMIN_INVITE',
            'PLATFORM_ADMIN_GRANTED',
            'CPO_STAFF_NEW_IDENTITY',
            'CPO_STAFF_EXISTING_IDENTITY',
            'CPO_ONBOARDING_RESENT',
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
    ) NOT VALID;
