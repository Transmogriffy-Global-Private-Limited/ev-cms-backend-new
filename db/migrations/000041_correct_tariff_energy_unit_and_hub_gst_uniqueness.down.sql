DROP INDEX IF EXISTS uq_hubs_cpo_gst_id;

ALTER TYPE units RENAME VALUE 'kwh' TO 'watt/hour';
