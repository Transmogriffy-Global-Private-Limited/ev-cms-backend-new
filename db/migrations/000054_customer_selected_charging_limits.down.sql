ALTER TABLE charging_start_intents
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_requested_limit,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_limit_type,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_energy_limit,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_duration,
    ADD CONSTRAINT chk_charging_start_intents_energy_limit CHECK (energy_limit_wh > 0),
    ADD CONSTRAINT chk_charging_start_intents_duration CHECK (max_duration_seconds > 0),
    DROP COLUMN IF EXISTS requested_limit_value,
    DROP COLUMN IF EXISTS limit_type;
