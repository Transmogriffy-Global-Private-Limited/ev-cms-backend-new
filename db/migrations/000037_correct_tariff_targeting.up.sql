-- A tariff has exactly one durable target. Older rows carried their hub as
-- mandatory context even for charger and user-group tariffs; normalize that
-- representation before enforcing the corrected model.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM tariffs
        WHERE hub_id IS NULL
          AND charger_id IS NULL
          AND user_group_id IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot normalize tariff with no target relationship'
            USING ERRCODE = '23514';
    END IF;
END $$;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS tariffs_active_effective_period_exclusion,
    DROP CONSTRAINT IF EXISTS fk_tariffs_charger,
    ALTER COLUMN hub_id DROP NOT NULL;

-- The declared legacy precedence is deliberate: a user-group target wins over
-- charger target, which wins over the old mandatory hub context.
UPDATE tariffs
SET hub_id = NULL,
    charger_id = NULL,
    assigned_to = 'usergroup'::tariff_assignment_type
WHERE user_group_id IS NOT NULL;

UPDATE tariffs
SET hub_id = NULL,
    assigned_to = 'charger'::tariff_assignment_type
WHERE user_group_id IS NULL
  AND charger_id IS NOT NULL;

UPDATE tariffs
SET assigned_to = 'hub'::tariff_assignment_type
WHERE user_group_id IS NULL
  AND charger_id IS NULL;

ALTER TABLE tariffs
    ALTER COLUMN assigned_to SET NOT NULL,
    ADD CONSTRAINT fk_tariffs_charger
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT tariffs_exactly_one_target CHECK (
        ((hub_id IS NOT NULL)::integer +
         (charger_id IS NOT NULL)::integer +
         (user_group_id IS NOT NULL)::integer) = 1
    ),
    ADD CONSTRAINT tariffs_target_matches_assigned_to CHECK (
        (assigned_to = 'hub'::tariff_assignment_type
            AND hub_id IS NOT NULL AND charger_id IS NULL AND user_group_id IS NULL)
        OR
        (assigned_to = 'charger'::tariff_assignment_type
            AND hub_id IS NULL AND charger_id IS NOT NULL AND user_group_id IS NULL)
        OR
        (assigned_to = 'usergroup'::tariff_assignment_type
            AND hub_id IS NULL AND charger_id IS NULL AND user_group_id IS NOT NULL)
    ),
    ADD CONSTRAINT tariffs_active_effective_period_exclusion
        EXCLUDE USING gist (
            cpo_id WITH =,
            COALESCE(hub_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            COALESCE(charger_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            COALESCE(user_group_id, '00000000-0000-0000-0000-000000000000'::uuid) WITH =,
            tstzrange(
                COALESCE(start_date, '-infinity'::timestamptz),
                COALESCE(end_date, 'infinity'::timestamptz),
                '[)'
            ) WITH &&
        ) WHERE (is_active);

DROP INDEX IF EXISTS ix_tariffs_cpo_hub_active;
CREATE INDEX ix_tariffs_cpo_hub_active
    ON tariffs (cpo_id, hub_id, is_active)
    WHERE hub_id IS NOT NULL;
