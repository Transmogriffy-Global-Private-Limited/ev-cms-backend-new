-- Customer intent and wallet safety are independent. Preserve the source of
-- each effective physical threshold rather than overloading limit_type.
ALTER TABLE charging_start_intents
    ADD COLUMN energy_limit_source varchar(32) NOT NULL DEFAULT 'NONE',
    ADD COLUMN duration_limit_source varchar(32) NOT NULL DEFAULT 'NONE';

UPDATE charging_start_intents
SET energy_limit_source = CASE
    WHEN energy_limit_wh = 0 THEN 'NONE'
    WHEN limit_type = 'ENERGY' THEN 'CUSTOMER_ENERGY'
    WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
    WHEN limit_type = 'AUTO' THEN 'WALLET'
    ELSE 'NONE'
END,
duration_limit_source = CASE
    WHEN max_duration_seconds = 0 THEN 'NONE'
    WHEN limit_type = 'TIME' THEN 'CUSTOMER_TIME'
    WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
    WHEN limit_type = 'AUTO' THEN 'WALLET'
    ELSE 'NONE'
END;

ALTER TABLE charging_start_intents
    ADD CONSTRAINT chk_charging_start_intents_energy_limit_source CHECK (energy_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_charging_start_intents_duration_limit_source CHECK (duration_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_charging_start_intents_energy_limit_provenance CHECK ((energy_limit_wh = 0 AND energy_limit_source = 'NONE') OR (energy_limit_wh > 0 AND energy_limit_source IN ('CUSTOMER_ENERGY','CUSTOMER_MONEY','WALLET'))),
    ADD CONSTRAINT chk_charging_start_intents_duration_limit_provenance CHECK ((max_duration_seconds = 0 AND duration_limit_source = 'NONE') OR (max_duration_seconds > 0 AND duration_limit_source IN ('CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')));
