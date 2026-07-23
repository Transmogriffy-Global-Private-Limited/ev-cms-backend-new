DELETE FROM mail_outbox
WHERE template = 'CUSTOMER_SIGNUP_OTP';

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER'
        )
    );

DROP TABLE IF EXISTS customer_signup_challenges;
