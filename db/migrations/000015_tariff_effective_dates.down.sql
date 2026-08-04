DROP INDEX IF EXISTS ix_tariffs_end_date;
DROP INDEX IF EXISTS ix_tariffs_start_date;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS tariffs_active_effective_period_exclusion,
    DROP CONSTRAINT IF EXISTS tariffs_effective_dates_check,
    DROP COLUMN IF EXISTS end_date,
    DROP COLUMN IF EXISTS start_date;
