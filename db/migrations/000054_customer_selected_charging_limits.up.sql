ALTER TABLE charging_start_intents
    ADD COLUMN limit_type varchar(16) NOT NULL DEFAULT 'AUTO',
    ADD COLUMN requested_limit_value numeric(14,3);

-- Energy, time, and fixed-session tariffs have different effective limits.
-- The old one-hour/positive-energy invariant cannot represent that truth.
ALTER TABLE charging_start_intents
    DROP CONSTRAINT chk_charging_start_intents_energy_limit,
    DROP CONSTRAINT chk_charging_start_intents_duration,
    ADD CONSTRAINT chk_charging_start_intents_energy_limit CHECK (energy_limit_wh >= 0),
    ADD CONSTRAINT chk_charging_start_intents_duration CHECK (max_duration_seconds >= 0);

ALTER TABLE charging_start_intents
    ADD CONSTRAINT chk_charging_start_intents_limit_type CHECK (limit_type IN ('AUTO', 'ENERGY', 'TIME', 'MONEY')),
    ADD CONSTRAINT chk_charging_start_intents_requested_limit CHECK (
        (limit_type = 'AUTO' AND requested_limit_value IS NULL) OR
        (limit_type <> 'AUTO' AND requested_limit_value IS NOT NULL AND requested_limit_value > 0)
    );
