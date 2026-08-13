ALTER TABLE chargers
    DROP CONSTRAINT IF EXISTS chargers_active_requires_hub,
    DROP CONSTRAINT IF EXISTS chargers_customer_visibility_requires_hub,
    ALTER COLUMN customer_visibility SET DEFAULT TRUE;

ALTER TABLE tariffs
    ADD COLUMN gst_id uuid,
    ADD CONSTRAINT fk_tariffs_gst
        FOREIGN KEY (cpo_id, gst_id)
        REFERENCES gsts(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT;
