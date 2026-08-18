-- Tariff amounts historically represent a commercial price per kWh. Rename
-- only the enum label; numeric price_per_unit values remain unchanged.
ALTER TYPE units RENAME VALUE 'watt/hour' TO 'kwh';

-- CPO writes already treat one GST profile as one Hub assignment. Preserve
-- that invariant under concurrent writes without choosing a winner for any
-- inconsistent existing deployment data.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM hubs
        WHERE gst_id IS NOT NULL
        GROUP BY cpo_id, gst_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce unique Hub GST assignment while duplicate assignments exist'
            USING ERRCODE = '23505';
    END IF;
END $$;

CREATE UNIQUE INDEX uq_hubs_cpo_gst_id
    ON hubs (cpo_id, gst_id)
    WHERE gst_id IS NOT NULL;
