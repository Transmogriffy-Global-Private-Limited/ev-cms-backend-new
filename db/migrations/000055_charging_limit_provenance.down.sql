ALTER TABLE charging_start_intents
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_duration_limit_provenance,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_energy_limit_provenance,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_duration_limit_source,
    DROP CONSTRAINT IF EXISTS chk_charging_start_intents_energy_limit_source,
    DROP COLUMN IF EXISTS duration_limit_source,
    DROP COLUMN IF EXISTS energy_limit_source;
