ALTER TABLE cpos
    ADD COLUMN status_reason varchar(500),
    ADD COLUMN status_changed_at timestamptz,
    ADD COLUMN status_changed_by_user_id uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL;

UPDATE cpos
SET status_reason = 'Initial provisioning',
    status_changed_at = updated_at
WHERE status_reason IS NULL
   OR status_changed_at IS NULL;

ALTER TABLE cpos
    ALTER COLUMN status_reason SET NOT NULL,
    ALTER COLUMN status_changed_at SET NOT NULL,
    ADD CONSTRAINT chk_cpos_status_reason CHECK (
        char_length(btrim(status_reason)) BETWEEN 3 AND 500
    );

CREATE INDEX ix_cpos_status_created
    ON cpos (status, created_at DESC, id DESC);

ALTER TABLE cpo_memberships
    ADD COLUMN is_primary_admin boolean NOT NULL DEFAULT false;

WITH ranked_admins AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY cpo_id
            ORDER BY
                CASE WHEN status = 'ACTIVE' THEN 0 ELSE 1 END,
                created_at,
                id
        ) AS position
    FROM cpo_memberships
    WHERE role IN ('OWNER', 'ADMIN')
)
UPDATE cpo_memberships AS membership
SET is_primary_admin = true
FROM ranked_admins
WHERE membership.id = ranked_admins.id
  AND ranked_admins.position = 1;

ALTER TABLE cpo_memberships
    ADD CONSTRAINT chk_cpo_memberships_primary_admin_role CHECK (
        NOT is_primary_admin OR role IN ('OWNER', 'ADMIN')
    );

CREATE UNIQUE INDEX uq_cpo_memberships_primary_admin
    ON cpo_memberships (cpo_id)
    WHERE is_primary_admin;

ALTER TABLE mail_outbox
    ADD COLUMN cpo_id uuid
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD COLUMN user_id uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL;

CREATE INDEX ix_mail_outbox_cpo_user_created
    ON mail_outbox (cpo_id, user_id, created_at DESC)
    WHERE cpo_id IS NOT NULL;

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
            'CPO_PLATFORM_INVOICE_ISSUED',
            'CPO_ONBOARDING_RESENT'
        )
    );
