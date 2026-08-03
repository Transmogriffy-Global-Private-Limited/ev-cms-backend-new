ALTER TABLE tariffs
ADD COLUMN start_date timestamptz,
ADD COLUMN end_date timestamptz;

CREATE INDEX ix_tariffs_start_date ON tariffs (start_date);
CREATE INDEX ix_tariffs_end_date ON tariffs (end_date);
