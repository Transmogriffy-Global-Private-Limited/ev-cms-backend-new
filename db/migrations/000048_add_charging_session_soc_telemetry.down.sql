ALTER TABLE charging_sessions
    DROP CONSTRAINT IF EXISTS chk_charging_sessions_soc_sequence,
    DROP CONSTRAINT IF EXISTS chk_charging_sessions_soc_observation,
    DROP CONSTRAINT IF EXISTS chk_charging_sessions_latest_soc_percent,
    DROP CONSTRAINT IF EXISTS chk_charging_sessions_initial_soc_percent,
    DROP COLUMN IF EXISTS soc_sequence,
    DROP COLUMN IF EXISTS soc_observed_at,
    DROP COLUMN IF EXISTS latest_soc_percent,
    DROP COLUMN IF EXISTS initial_soc_percent;
