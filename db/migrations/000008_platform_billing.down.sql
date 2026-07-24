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
            'CPO_SUBSCRIPTION_CHANGED'
        )
    );

DROP TRIGGER IF EXISTS trg_platform_invoice_lines_protect_issued
    ON platform_invoice_lines;
DROP FUNCTION IF EXISTS protect_platform_invoice_line();
DROP TRIGGER IF EXISTS trg_platform_invoices_protect_issued
    ON platform_invoices;
DROP FUNCTION IF EXISTS protect_issued_platform_invoice();

DROP TABLE IF EXISTS platform_payment_allocations;
DROP TABLE IF EXISTS platform_payments;
DROP TABLE IF EXISTS platform_invoice_lines;
DROP TABLE IF EXISTS platform_invoices;
DROP TABLE IF EXISTS cpo_billing_accounts;
