ALTER TABLE tariffs
ADD COLUMN start_date timestamptz,
ADD COLUMN end_date timestamptz,
ADD CONSTRAINT tariffs_date_range_check CHECK (
    (start_date IS NULL AND end_date IS NULL) OR
    (start_date IS NOT NULL AND end_date IS NOT NULL AND start_date < end_date)
);

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE tariffs ADD CONSTRAINT tariffs_scope_overlap_exclusive EXCLUDE USING gist (cpo_id WITH =, hub_id WITH =, charger_id WITH =, user_group_id WITH =, tsrange(start_date, end_date) WITH &&) WHERE (is_active);

CREATE INDEX ix_tariffs_start_date ON tariffs (start_date);
CREATE INDEX ix_tariffs_end_date ON tariffs (end_date);
