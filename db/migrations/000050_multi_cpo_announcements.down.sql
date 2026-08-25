ALTER TABLE platform_notifications
    DROP CONSTRAINT IF EXISTS uq_platform_notifications_recipient_scope,
    ADD CONSTRAINT uq_platform_notifications_recipient UNIQUE (announcement_id, recipient_user_id);

DROP TABLE IF EXISTS platform_announcement_cpos;

ALTER TABLE platform_announcements
    DROP CONSTRAINT IF EXISTS chk_platform_announcements_audience,
    ADD CONSTRAINT chk_platform_announcements_audience CHECK (
        (audience = 'PLATFORM' AND cpo_id IS NULL)
        OR (audience = 'CPO' AND cpo_id IS NOT NULL)
    );
