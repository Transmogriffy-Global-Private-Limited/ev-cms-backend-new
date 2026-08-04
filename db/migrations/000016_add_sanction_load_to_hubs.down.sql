DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM chargers WHERE hub_id IS NULL) THEN
        RAISE EXCEPTION 'cannot restore chargers.hub_id NOT NULL while independent chargers exist';
    END IF;
END;
$$;

ALTER TABLE chargers
    ALTER COLUMN hub_id SET NOT NULL;

ALTER TABLE hubs DROP COLUMN sanction_load;
