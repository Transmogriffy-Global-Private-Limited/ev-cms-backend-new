-- The former exclusion model cannot represent open-ended fallbacks or nested
-- bounded overrides. Refuse rollback when data would be silently discarded.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tariffs WHERE is_active AND start_date IS NOT NULL AND end_date IS NULL) THEN
        RAISE EXCEPTION 'cannot roll back temporal tariff fallback while enabled open-ended tariffs exist' USING ERRCODE = '23514';
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
    ) THEN
        RAISE EXCEPTION 'cannot roll back temporal tariff fallback while enabled bounded overrides overlap' USING ERRCODE = '23514';
    END IF;
END $$;

DROP TRIGGER IF EXISTS hubs_customer_visible_tariff_root_guard ON hubs;
DROP FUNCTION IF EXISTS guard_customer_visible_hub_tariff_root();
DROP TRIGGER IF EXISTS tariffs_temporal_target_guard ON tariffs;
DROP FUNCTION IF EXISTS validate_temporal_tariff_target();
DROP INDEX IF EXISTS ix_tariffs_enabled_target_temporal;
DROP INDEX IF EXISTS uq_tariffs_enabled_target_open_start;
DROP INDEX IF EXISTS uq_tariffs_enabled_target_root;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS tariffs_temporal_dates_check,
    ADD CONSTRAINT tariffs_effective_dates_check CHECK (
        (start_date IS NULL AND end_date IS NULL)
        OR (start_date IS NOT NULL AND end_date IS NOT NULL AND start_date < end_date)
    ),
    ADD CONSTRAINT tariffs_active_effective_period_exclusion
        EXCLUDE USING gist (
            cpo_id WITH =,
            COALESCE(hub_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            COALESCE(charger_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            tstzrange(COALESCE(start_date, '-infinity'::timestamptz), COALESCE(end_date, 'infinity'::timestamptz), '[)') WITH &&
        ) WHERE (is_active);
