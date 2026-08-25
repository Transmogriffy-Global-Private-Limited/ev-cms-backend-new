ALTER TABLE charging_sessions
    ADD COLUMN initial_soc_percent numeric(6,3),
    ADD COLUMN latest_soc_percent numeric(6,3),
    ADD COLUMN soc_observed_at timestamptz,
    ADD COLUMN soc_sequence bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_charging_sessions_initial_soc_percent CHECK (initial_soc_percent IS NULL OR (initial_soc_percent >= 0 AND initial_soc_percent <= 100)),
    ADD CONSTRAINT chk_charging_sessions_latest_soc_percent CHECK (latest_soc_percent IS NULL OR (latest_soc_percent >= 0 AND latest_soc_percent <= 100)),
    ADD CONSTRAINT chk_charging_sessions_soc_observation CHECK ((latest_soc_percent IS NULL AND soc_observed_at IS NULL) OR (latest_soc_percent IS NOT NULL AND soc_observed_at IS NOT NULL)),
    ADD CONSTRAINT chk_charging_sessions_soc_sequence CHECK (soc_sequence >= 0);
