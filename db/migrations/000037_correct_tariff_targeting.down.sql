-- The old schema cannot represent charger-only or user-group-only tariffs.
-- Refuse rollback rather than deleting data or inventing a hub relationship.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM tariffs
        WHERE assigned_to <> 'hub'::tariff_assignment_type
           OR hub_id IS NULL
           OR charger_id IS NOT NULL
           OR user_group_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot roll back tariff targeting while non-hub tariffs exist'
            USING ERRCODE = '23514';
    END IF;
END $$;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS tariffs_active_effective_period_exclusion,
    DROP CONSTRAINT IF EXISTS tariffs_target_matches_assigned_to,
    DROP CONSTRAINT IF EXISTS tariffs_exactly_one_target,
    DROP CONSTRAINT IF EXISTS fk_tariffs_charger,
    ALTER COLUMN assigned_to DROP NOT NULL,
    ALTER COLUMN hub_id SET NOT NULL,
    ADD CONSTRAINT fk_tariffs_charger
        FOREIGN KEY (cpo_id, hub_id, charger_id)
        REFERENCES chargers(cpo_id, hub_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT tariffs_active_effective_period_exclusion
        EXCLUDE USING gist (
            cpo_id WITH =,
            hub_id WITH =,
            COALESCE(charger_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            tstzrange(
                COALESCE(start_date, '-infinity'::timestamptz),
                COALESCE(end_date, 'infinity'::timestamptz),
                '[)'
            ) WITH &&
        ) WHERE (is_active);

DROP INDEX IF EXISTS ix_tariffs_cpo_hub_active;
CREATE INDEX ix_tariffs_cpo_hub_active ON tariffs (cpo_id, hub_id, is_active);
