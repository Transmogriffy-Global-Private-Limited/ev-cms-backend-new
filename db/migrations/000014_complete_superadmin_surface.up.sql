ALTER TABLE platform_admins
    ADD COLUMN is_active boolean NOT NULL DEFAULT true,
    ADD COLUMN status_reason varchar(500) NOT NULL DEFAULT 'Initial platform authority',
    ADD COLUMN status_changed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN status_changed_by_user_id uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
    ADD CONSTRAINT chk_platform_admin_status_reason CHECK (
        char_length(btrim(status_reason)) BETWEEN 3 AND 500
    );

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    DROP CONSTRAINT chk_mail_outbox_status,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP', 'PASSWORD_RESET_OTP', 'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED', 'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP', 'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP', 'CPO_ONBOARDING_RESENT',
            'CPO_SUBSCRIPTION_CHANGED', 'CPO_PLATFORM_INVOICE_ISSUED',
            'PLATFORM_ADMIN_INVITE', 'PLATFORM_ADMIN_GRANTED'
        )
    ),
    ADD CONSTRAINT chk_mail_outbox_status CHECK (
        status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED', 'CANCELED')
    );

CREATE TABLE platform_announcements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    audience varchar(20) NOT NULL,
    cpo_id uuid REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    title varchar(200) NOT NULL,
    body text NOT NULL,
    created_by_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    CONSTRAINT chk_platform_announcements_audience CHECK (
        (audience = 'PLATFORM' AND cpo_id IS NULL)
        OR (audience = 'CPO' AND cpo_id IS NOT NULL)
    ),
    CONSTRAINT chk_platform_announcements_title CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
    CONSTRAINT chk_platform_announcements_body CHECK (char_length(btrim(body)) BETWEEN 1 AND 10000),
    CONSTRAINT chk_platform_announcements_expiry CHECK (expires_at IS NULL OR expires_at > created_at)
);

CREATE INDEX ix_platform_announcements_created
    ON platform_announcements (created_at DESC, id DESC);
CREATE INDEX ix_platform_announcements_cpo_created
    ON platform_announcements (cpo_id, created_at DESC, id DESC)
    WHERE cpo_id IS NOT NULL;

CREATE TABLE platform_notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    announcement_id uuid NOT NULL REFERENCES platform_announcements(id) ON UPDATE CASCADE ON DELETE CASCADE,
    recipient_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    cpo_id uuid REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    CONSTRAINT uq_platform_notifications_recipient UNIQUE (announcement_id, recipient_user_id)
);

CREATE INDEX ix_platform_notifications_recipient_created
    ON platform_notifications (recipient_user_id, created_at DESC, id DESC);
CREATE INDEX ix_platform_notifications_cpo_recipient
    ON platform_notifications (cpo_id, recipient_user_id, created_at DESC)
    WHERE cpo_id IS NOT NULL;
