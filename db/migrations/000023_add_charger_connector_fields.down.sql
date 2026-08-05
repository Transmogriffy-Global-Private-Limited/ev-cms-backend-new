ALTER TABLE chargers
    DROP COLUMN charger_name,
    DROP COLUMN charger_host_name,
    DROP COLUMN charger_host_phone_no,
    DROP COLUMN charger_type,
    DROP COLUMN segment,
    DROP COLUMN sub_segment,
    DROP COLUMN total_capacity,
    DROP COLUMN charger_image,
    DROP COLUMN charger_use_type,
    DROP COLUMN number_of_connectors,
    DROP COLUMN parking,
    DROP COLUMN protocol,
    DROP COLUMN twenty_four_seven_open_status;

ALTER TABLE connectors
    DROP COLUMN connector_max_capacity;
