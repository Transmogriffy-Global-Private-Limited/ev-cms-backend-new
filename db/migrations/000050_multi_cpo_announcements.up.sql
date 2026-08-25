ALTER TABLE platform_announcements
    DROP CONSTRAINT chk_platform_announcements_audience,
    ADD CONSTRAINT chk_platform_announcements_audience CHECK (
        (audience = 'PLATFORM' AND cpo_id IS NULL)
        OR audience = 'CPO'
    );

CREATE TABLE platform_announcement_cpos (
    announcement_id uuid NOT NULL REFERENCES platform_announcements(id) ON UPDATE CASCADE ON DELETE CASCADE,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (announcement_id, cpo_id)
);

INSERT INTO platform_announcement_cpos (announcement_id, cpo_id, created_at)
SELECT id, cpo_id, created_at FROM platform_announcements
WHERE audience = 'CPO' AND cpo_id IS NOT NULL
ON CONFLICT DO NOTHING;

ALTER TABLE platform_notifications
    DROP CONSTRAINT uq_platform_notifications_recipient,
    ADD CONSTRAINT uq_platform_notifications_recipient_scope UNIQUE (announcement_id, recipient_user_id, cpo_id);
