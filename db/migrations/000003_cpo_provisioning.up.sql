ALTER TABLE users
    ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;

ALTER TABLE cpos
    ADD COLUMN app_id varchar(100),
    ADD COLUMN app_id_mode varchar(20) NOT NULL DEFAULT 'DUMMY',
    ADD COLUMN app_id_updated_at timestamptz NOT NULL DEFAULT now();

UPDATE cpos
SET app_id = 'cpo_dummy_' || replace(gen_random_uuid()::text, '-', '')
WHERE app_id IS NULL;

ALTER TABLE cpos
    ALTER COLUMN app_id SET NOT NULL,
    ADD CONSTRAINT chk_cpos_app_id_format CHECK (
        char_length(app_id) BETWEEN 16 AND 100
        AND app_id ~ '^[a-z0-9_-]+$'
    ),
    ADD CONSTRAINT chk_cpos_app_id_mode CHECK (
        (app_id_mode = 'DUMMY' AND app_id LIKE 'cpo_dummy_%')
        OR
        (app_id_mode = 'LIVE' AND app_id NOT LIKE 'cpo_dummy_%')
    );

CREATE UNIQUE INDEX uq_cpos_app_id ON cpos (app_id);

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
