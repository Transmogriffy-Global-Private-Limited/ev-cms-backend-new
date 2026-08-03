DROP INDEX IF EXISTS ix_tariffs_end_date;
DROP INDEX IF EXISTS ix_tariffs_start_date;

ALTER TABLE tariffs
DROP COLUMN IF EXISTS end_date,
DROP COLUMN IF EXISTS start_date;
