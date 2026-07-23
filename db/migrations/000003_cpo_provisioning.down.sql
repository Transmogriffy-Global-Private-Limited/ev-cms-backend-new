DELETE FROM mail_outbox
WHERE template IN (
    'CPO_ADMIN_WELCOME',
    'CPO_MEMBERSHIP_ASSIGNED',
    'PASSWORD_CHANGE_REMINDER'
);

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN ('LOGIN_OTP', 'PASSWORD_RESET_OTP')
    );

DROP INDEX IF EXISTS uq_cpos_app_id;

ALTER TABLE cpos
    DROP CONSTRAINT IF EXISTS chk_cpos_app_id_mode,
    DROP CONSTRAINT IF EXISTS chk_cpos_app_id_format,
    DROP COLUMN IF EXISTS app_id_updated_at,
    DROP COLUMN IF EXISTS app_id_mode,
    DROP COLUMN IF EXISTS app_id;

ALTER TABLE users
    DROP COLUMN IF EXISTS must_change_password;
