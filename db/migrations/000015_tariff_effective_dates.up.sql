ALTER TABLE tariffs
    ADD COLUMN start_date timestamptz,
    ADD COLUMN end_date timestamptz,
    ADD CONSTRAINT tariffs_effective_dates_check CHECK (
        (start_date IS NULL AND end_date IS NULL) OR
        (start_date IS NOT NULL AND end_date IS NOT NULL AND start_date < end_date)
    );

CREATE EXTENSION IF NOT EXISTS btree_gist;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM tariffs AS older
        JOIN tariffs AS newer
          ON newer.id > older.id
         AND newer.cpo_id = older.cpo_id
         AND newer.hub_id = older.hub_id
         AND newer.charger_id IS NOT DISTINCT FROM older.charger_id
         AND newer.user_group_id IS NOT DISTINCT FROM older.user_group_id
         AND tstzrange(
                COALESCE(newer.start_date, '-infinity'::timestamptz),
                COALESCE(newer.end_date, 'infinity'::timestamptz),
                '[)'
             ) && tstzrange(
                COALESCE(older.start_date, '-infinity'::timestamptz),
                COALESCE(older.end_date, 'infinity'::timestamptz),
                '[)'
             )
        WHERE older.is_active
          AND newer.is_active
    ) THEN
        RAISE EXCEPTION
            'cannot add tariff effective-date constraint while overlapping active tariffs exist'
            USING ERRCODE = '23514';
    END IF;
END $$;

ALTER TABLE tariffs
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

CREATE INDEX ix_tariffs_start_date ON tariffs (start_date);
CREATE INDEX ix_tariffs_end_date ON tariffs (end_date);
