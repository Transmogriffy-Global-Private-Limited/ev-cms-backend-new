-- Tariffs are commercial-only. GST is selected solely from the charger's hub.
-- Existing tariff GST associations cannot be safely inferred as hub tax, so
-- stop before altering any data if a deployment still depends on one.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tariffs WHERE gst_id IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot remove tariffs.gst_id while tariff GST associations exist'
            USING ERRCODE = '23514';
    END IF;
END $$;

ALTER TABLE tariffs
    DROP CONSTRAINT IF EXISTS fk_tariffs_gst,
    DROP COLUMN gst_id;

-- Provisioning may create hubless chargers, but it may not publish or activate
-- them. Normalize pre-existing rows before the durable invariants are added.
UPDATE chargers
SET customer_visibility = FALSE,
    updated_at = now()
WHERE hub_id IS NULL
  AND customer_visibility;

UPDATE chargers
SET status = 'INACTIVE',
    updated_at = now()
WHERE hub_id IS NULL
  AND status = 'ACTIVE';

ALTER TABLE chargers
    ALTER COLUMN customer_visibility SET DEFAULT FALSE,
    ADD CONSTRAINT chargers_customer_visibility_requires_hub
        CHECK (NOT customer_visibility OR hub_id IS NOT NULL),
    ADD CONSTRAINT chargers_active_requires_hub
        CHECK (status <> 'ACTIVE' OR hub_id IS NOT NULL);
