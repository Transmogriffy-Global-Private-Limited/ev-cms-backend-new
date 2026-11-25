-- Forward-only repair of already-published 000035 and 000050 semantics.
-- No historical rows are deleted, merged, reassigned or rewritten.
-- This script runs as ONE transaction under db/migration.go.

-- 000035: An assigned legal/tax profile cannot be deleted by making the
-- mandatory tenant identity NULL. Existing APIs explicitly unassign GST from
-- a Hub first. Preserve that deliberate flow, prohibit physical deletion of
-- a still-assigned tax profile, and retain same-CPO ownership.
ALTER TABLE hubs DROP CONSTRAINT fk_hubs_gst;
ALTER TABLE hubs
    ADD CONSTRAINT fk_hubs_gst
    FOREIGN KEY (cpo_id, gst_id) REFERENCES gsts(cpo_id, id)
    ON UPDATE NO ACTION ON DELETE RESTRICT
    NOT VALID;
ALTER TABLE hubs VALIDATE CONSTRAINT fk_hubs_gst;

-- 000050: A NULL CPO identifies a PLATFORM recipient. PostgreSQL's normal
-- multi-column UNIQUE treats NULL values as distinct, so it does not prevent
-- duplicate PLATFORM notifications. Fail without modifying existing rows.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM platform_notifications
         WHERE cpo_id IS NULL
         GROUP BY announcement_id, recipient_user_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION '000078: duplicate platform notifications; reconcile original recipient/read history before continuing'
            USING ERRCODE = '23505';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM platform_notifications AS n
          JOIN platform_announcements AS a ON a.id = n.announcement_id
         WHERE (a.audience = 'PLATFORM' AND n.cpo_id IS NOT NULL)
            OR (a.audience = 'CPO' AND n.cpo_id IS NULL)
    ) THEN
        RAISE EXCEPTION '000078: platform notification audience/scope mismatch; review records without rewriting them'
            USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM platform_notifications AS n
          JOIN platform_announcements AS a ON a.id = n.announcement_id
         WHERE a.audience = 'CPO'
           AND NOT EXISTS (
                SELECT 1 FROM platform_announcement_cpos AS target
                 WHERE target.announcement_id = n.announcement_id
                   AND target.cpo_id = n.cpo_id
           )
    ) THEN
        RAISE EXCEPTION '000078: CPO notification has no matching announcement target; review source evidence'
            USING ERRCODE = '23503';
    END IF;
END;
$$;

CREATE UNIQUE INDEX uq_platform_notifications_platform_recipient
    ON platform_notifications (announcement_id, recipient_user_id)
    WHERE cpo_id IS NULL;

-- Enforce the announced CPO target as an actual compound FK. The deferred
-- NO ACTION protects the usual parent-announcement DELETE CASCADE fan-out:
-- notification and target rows may be removed in either trigger order.
ALTER TABLE platform_notifications
    ADD CONSTRAINT fk_platform_notifications_announcement_target
    FOREIGN KEY (announcement_id, cpo_id)
    REFERENCES platform_announcement_cpos(announcement_id, cpo_id)
    ON UPDATE RESTRICT ON DELETE NO ACTION
    DEFERRABLE INITIALLY DEFERRED
    NOT VALID;
ALTER TABLE platform_notifications
    VALIDATE CONSTRAINT fk_platform_notifications_announcement_target;

-- A check constraint cannot inspect the parent announcement's audience.
-- This narrow trigger enforces that cross-row invariant on INSERT/UPDATE.
CREATE FUNCTION guard_platform_notification_scope()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    announcement_audience varchar(20);
BEGIN
    SELECT audience INTO announcement_audience
      FROM platform_announcements
     WHERE id = NEW.announcement_id
     FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'notification announcement does not exist'
            USING ERRCODE = '23503';
    END IF;
    IF (announcement_audience = 'PLATFORM' AND NEW.cpo_id IS NOT NULL)
       OR (announcement_audience = 'CPO' AND NEW.cpo_id IS NULL) THEN
        RAISE EXCEPTION 'notification CPO scope does not match announcement audience'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER platform_notification_scope_guard
BEFORE INSERT OR UPDATE OF announcement_id, cpo_id
ON platform_notifications
FOR EACH ROW EXECUTE FUNCTION guard_platform_notification_scope();

-- Existing announcements are immutable once they have recipients; otherwise
-- an audience edit could invalidate a valid notification after insertion.
CREATE FUNCTION guard_platform_announcement_audience()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.audience IS DISTINCT FROM OLD.audience
       AND EXISTS (
           SELECT 1 FROM platform_notifications
            WHERE announcement_id = OLD.id
       ) THEN
        RAISE EXCEPTION 'cannot change announcement audience after notifications exist'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER platform_announcement_audience_guard
BEFORE UPDATE OF audience ON platform_announcements
FOR EACH ROW EXECUTE FUNCTION guard_platform_announcement_audience();
