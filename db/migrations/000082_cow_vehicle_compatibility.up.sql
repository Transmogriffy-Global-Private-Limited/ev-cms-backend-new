-- Run 2B: additive CPO/customer-scoped vehicle compatibility declarations.
-- Existing rows retain null/unknown compatibility; never infer specs from make/model.
ALTER TABLE vehicles
    ADD COLUMN battery_capacity_wh bigint,
    ADD COLUMN connector_types jsonb,
    ADD COLUMN supports_ac boolean,
    ADD COLUMN supports_dc boolean,
    ADD COLUMN max_ac_power_w bigint,
    ADD COLUMN max_dc_power_w bigint,
    ADD COLUMN version bigint NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_cow_vehicle_battery_capacity_positive
        CHECK (battery_capacity_wh IS NULL OR battery_capacity_wh > 0),
    ADD CONSTRAINT chk_cow_vehicle_ac_power_positive
        CHECK (max_ac_power_w IS NULL OR max_ac_power_w > 0),
    ADD CONSTRAINT chk_cow_vehicle_dc_power_positive
        CHECK (max_dc_power_w IS NULL OR max_dc_power_w > 0),
    ADD CONSTRAINT chk_cow_vehicle_mode_consistency
        CHECK ((supports_ac IS DISTINCT FROM FALSE OR max_ac_power_w IS NULL)
           AND (supports_dc IS DISTINCT FROM FALSE OR max_dc_power_w IS NULL)),
    ADD CONSTRAINT chk_cow_vehicle_connector_array
        CHECK (connector_types IS NULL OR jsonb_typeof(connector_types) = 'null' OR
               (jsonb_typeof(connector_types) = 'array' AND jsonb_array_length(connector_types) BETWEEN 1 AND 8)),
    ADD CONSTRAINT chk_cow_vehicle_version_positive CHECK (version >= 1);
