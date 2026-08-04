ALTER TABLE hubs
    ADD COLUMN sanction_load NUMERIC(10, 2) NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_hubs_sanction_load CHECK (sanction_load >= 0);

ALTER TABLE chargers
    ALTER COLUMN hub_id DROP NOT NULL;
