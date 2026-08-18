-- An enabled tariff target is a temporal fallback hierarchy, not a set of
-- mutually exclusive effective periods. A root has no dates; an open fallback
-- has only start_date; a bounded override has [start_date, end_date).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tariffs WHERE start_date IS NULL AND end_date IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot enable temporal tariff fallback while end-only tariff schedules exist' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM tariffs
        WHERE is_active AND start_date IS NULL AND end_date IS NULL
        GROUP BY cpo_id, assigned_to, hub_id, charger_id, user_group_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enable temporal tariff fallback while an exact target has multiple enabled roots' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM tariffs
        WHERE is_active AND start_date IS NOT NULL AND end_date IS NULL
        GROUP BY cpo_id, assigned_to, hub_id, charger_id, user_group_id, start_date
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enable temporal tariff fallback while an exact target has duplicate enabled open-ended starts' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM tariffs AS first_tariff
        JOIN tariffs AS second_tariff
          ON second_tariff.id > first_tariff.id
         AND second_tariff.cpo_id = first_tariff.cpo_id
         AND second_tariff.assigned_to = first_tariff.assigned_to
         AND second_tariff.hub_id IS NOT DISTINCT FROM first_tariff.hub_id
         AND second_tariff.charger_id IS NOT DISTINCT FROM first_tariff.charger_id
         AND second_tariff.user_group_id IS NOT DISTINCT FROM first_tariff.user_group_id
        WHERE first_tariff.is_active AND second_tariff.is_active
          AND first_tariff.start_date IS NOT NULL AND first_tariff.end_date IS NOT NULL
          AND second_tariff.start_date IS NOT NULL AND second_tariff.end_date IS NOT NULL
          AND first_tariff.start_date < second_tariff.end_date
          AND second_tariff.start_date < first_tariff.end_date
          AND NOT (
              first_tariff.start_date <= second_tariff.start_date
              AND second_tariff.end_date <= first_tariff.end_date
              AND (first_tariff.start_date < second_tariff.start_date OR second_tariff.end_date < first_tariff.end_date)
          )
          AND NOT (
              second_tariff.start_date <= first_tariff.start_date
              AND first_tariff.end_date <= second_tariff.end_date
              AND (second_tariff.start_date < first_tariff.start_date OR first_tariff.end_date < second_tariff.end_date)
          )
    ) THEN
        RAISE EXCEPTION 'cannot enable temporal tariff fallback while enabled bounded overrides cross or are identical' USING ERRCODE = '23514';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM hubs
        WHERE customer_visible
          AND NOT EXISTS (
              SELECT 1 FROM tariffs
              WHERE tariffs.cpo_id = hubs.cpo_id
                AND tariffs.assigned_to = 'hub'::tariff_assignment_type
                AND tariffs.hub_id = hubs.id
                AND tariffs.is_active
                AND tariffs.start_date IS NULL
                AND tariffs.end_date IS NULL
          )
    ) THEN
        RAISE EXCEPTION 'cannot enable temporal tariff fallback while a customer-visible hub lacks an enabled root tariff' USING ERRCODE = '23514';
    END IF;
END $$;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS tariffs_active_effective_period_exclusion,
    DROP CONSTRAINT IF EXISTS tariffs_effective_dates_check,
    ADD CONSTRAINT tariffs_temporal_dates_check CHECK (
        (start_date IS NULL AND end_date IS NULL)
        OR (start_date IS NOT NULL AND end_date IS NULL)
        OR (start_date IS NOT NULL AND end_date IS NOT NULL AND start_date < end_date)
    );

CREATE UNIQUE INDEX uq_tariffs_enabled_target_root
    ON tariffs (cpo_id, assigned_to, COALESCE(hub_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(charger_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE is_active AND start_date IS NULL AND end_date IS NULL;

CREATE UNIQUE INDEX uq_tariffs_enabled_target_open_start
    ON tariffs (cpo_id, assigned_to, COALESCE(hub_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(charger_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid), start_date)
    WHERE is_active AND start_date IS NOT NULL AND end_date IS NULL;

CREATE INDEX ix_tariffs_enabled_target_temporal
    ON tariffs (cpo_id, assigned_to, hub_id, charger_id, user_group_id, start_date, end_date)
    WHERE is_active;

CREATE OR REPLACE FUNCTION guard_tariff_target_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.cpo_id IS DISTINCT FROM OLD.cpo_id
       OR NEW.assigned_to IS DISTINCT FROM OLD.assigned_to
       OR NEW.hub_id IS DISTINCT FROM OLD.hub_id
       OR NEW.charger_id IS DISTINCT FROM OLD.charger_id
       OR NEW.user_group_id IS DISTINCT FROM OLD.user_group_id THEN
        RAISE EXCEPTION 'tariff target is immutable after creation' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tariffs_target_immutable_guard
BEFORE UPDATE ON tariffs
FOR EACH ROW EXECUTE FUNCTION guard_tariff_target_immutable();

CREATE OR REPLACE FUNCTION validate_temporal_tariff_target()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    target_cpo_id uuid;
    target_assignment tariff_assignment_type;
    target_hub_id uuid;
    target_charger_id uuid;
    target_user_group_id uuid;
BEGIN
    target_cpo_id := COALESCE(NEW.cpo_id, OLD.cpo_id);
    target_assignment := COALESCE(NEW.assigned_to, OLD.assigned_to);
    target_hub_id := COALESCE(NEW.hub_id, OLD.hub_id);
    target_charger_id := COALESCE(NEW.charger_id, OLD.charger_id);
    target_user_group_id := COALESCE(NEW.user_group_id, OLD.user_group_id);
    PERFORM pg_advisory_xact_lock(hashtextextended('tariff:' || target_cpo_id::text || ':' || target_assignment::text || ':' || COALESCE(target_hub_id::text, target_charger_id::text, target_user_group_id::text), 0));

    IF EXISTS (
        SELECT 1
        FROM tariffs AS first_tariff
        JOIN tariffs AS second_tariff
          ON second_tariff.id > first_tariff.id
         AND second_tariff.cpo_id = first_tariff.cpo_id
         AND second_tariff.assigned_to = first_tariff.assigned_to
         AND second_tariff.hub_id IS NOT DISTINCT FROM first_tariff.hub_id
         AND second_tariff.charger_id IS NOT DISTINCT FROM first_tariff.charger_id
         AND second_tariff.user_group_id IS NOT DISTINCT FROM first_tariff.user_group_id
        WHERE first_tariff.cpo_id = target_cpo_id
          AND first_tariff.assigned_to = target_assignment
          AND first_tariff.hub_id IS NOT DISTINCT FROM target_hub_id
          AND first_tariff.charger_id IS NOT DISTINCT FROM target_charger_id
          AND first_tariff.user_group_id IS NOT DISTINCT FROM target_user_group_id
          AND first_tariff.is_active AND second_tariff.is_active
          AND first_tariff.start_date IS NOT NULL AND first_tariff.end_date IS NOT NULL
          AND second_tariff.start_date IS NOT NULL AND second_tariff.end_date IS NOT NULL
          AND first_tariff.start_date < second_tariff.end_date
          AND second_tariff.start_date < first_tariff.end_date
          AND NOT (
              first_tariff.start_date <= second_tariff.start_date
              AND second_tariff.end_date <= first_tariff.end_date
              AND (first_tariff.start_date < second_tariff.start_date OR second_tariff.end_date < first_tariff.end_date)
          )
          AND NOT (
              second_tariff.start_date <= first_tariff.start_date
              AND first_tariff.end_date <= second_tariff.end_date
              AND (second_tariff.start_date < first_tariff.start_date OR first_tariff.end_date < second_tariff.end_date)
          )
    ) THEN
        RAISE EXCEPTION 'enabled bounded tariff overrides must be strictly nested' USING ERRCODE = '23P01';
    END IF;

    IF target_assignment = 'hub'::tariff_assignment_type
       AND EXISTS (SELECT 1 FROM hubs WHERE id = target_hub_id AND cpo_id = target_cpo_id AND customer_visible)
       AND NOT EXISTS (
           SELECT 1 FROM tariffs
           WHERE cpo_id = target_cpo_id AND assigned_to = 'hub'::tariff_assignment_type
             AND hub_id = target_hub_id AND is_active AND start_date IS NULL AND end_date IS NULL
       ) THEN
        RAISE EXCEPTION 'customer-visible hub requires one enabled unbounded hub tariff' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER tariffs_temporal_target_guard
AFTER INSERT OR UPDATE OR DELETE ON tariffs
DEFERRABLE INITIALLY IMMEDIATE
FOR EACH ROW EXECUTE FUNCTION validate_temporal_tariff_target();

CREATE OR REPLACE FUNCTION guard_customer_visible_hub_tariff_root()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.customer_visible THEN
        PERFORM pg_advisory_xact_lock(hashtextextended('tariff:' || NEW.cpo_id::text || ':hub:' || NEW.id::text, 0));
        IF NOT EXISTS (
            SELECT 1 FROM tariffs
            WHERE cpo_id = NEW.cpo_id AND assigned_to = 'hub'::tariff_assignment_type
              AND hub_id = NEW.id AND is_active AND start_date IS NULL AND end_date IS NULL
        ) THEN
            RAISE EXCEPTION 'customer-visible hub requires one enabled unbounded hub tariff' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER hubs_customer_visible_tariff_root_guard
BEFORE INSERT OR UPDATE OF customer_visible ON hubs
FOR EACH ROW EXECUTE FUNCTION guard_customer_visible_hub_tariff_root();
