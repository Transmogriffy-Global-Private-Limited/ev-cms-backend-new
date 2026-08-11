ALTER TABLE charging_sessions
    ALTER COLUMN transaction_id TYPE bigint;

ALTER TABLE charging_sessions
    ADD COLUMN start_intent_id uuid,
    ADD COLUMN hal_transaction_id uuid,
    ADD COLUMN latest_meter_wh bigint,
    ADD COLUMN meter_observed_at timestamptz,
    ADD COLUMN meter_sequence bigint NOT NULL DEFAULT 0,
    ADD COLUMN settlement_status varchar(32) NOT NULL DEFAULT 'PENDING';

CREATE UNIQUE INDEX uq_charging_sessions_start_intent
    ON charging_sessions (start_intent_id)
    WHERE start_intent_id IS NOT NULL;
CREATE UNIQUE INDEX uq_charging_sessions_hal_transaction
    ON charging_sessions (hal_transaction_id)
    WHERE hal_transaction_id IS NOT NULL;

CREATE TABLE charging_start_intents (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    customer_id uuid NOT NULL REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    charger_id uuid NOT NULL REFERENCES chargers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    connector_id uuid NOT NULL REFERENCES connectors(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    wallet_id uuid NOT NULL REFERENCES wallets(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    tariff_id uuid NOT NULL REFERENCES tariffs(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    status varchar(32) NOT NULL,
    credential_hash char(64) NOT NULL,
    credential_expires_at timestamptz NOT NULL,
    command_expires_at timestamptz NOT NULL,
    energy_limit_wh bigint NOT NULL,
    max_duration_seconds bigint NOT NULL,
    tariff_snapshot jsonb NOT NULL,
    tax_snapshot jsonb NOT NULL,
    materialized_session_id uuid,
    hal_command_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_charging_start_intents_status CHECK (status IN (
        'REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED',
        'ACTUALLY_STARTED', 'REJECTED', 'EXPIRED', 'RECONCILIATION_REQUIRED'
    )),
    CONSTRAINT chk_charging_start_intents_energy_limit CHECK (energy_limit_wh > 0),
    CONSTRAINT chk_charging_start_intents_duration CHECK (max_duration_seconds > 0),
    CONSTRAINT chk_charging_start_intents_expiry CHECK (command_expires_at >= credential_expires_at)
);
CREATE UNIQUE INDEX uq_charging_start_intents_credential_hash ON charging_start_intents (credential_hash);
CREATE INDEX ix_charging_start_intents_customer_status ON charging_start_intents (cpo_id, customer_id, status, created_at DESC);
CREATE UNIQUE INDEX uq_charging_start_intents_open_connector
    ON charging_start_intents (connector_id)
    WHERE status IN ('REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED', 'ACTUALLY_STARTED', 'RECONCILIATION_REQUIRED');

CREATE TABLE wallet_holds (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    wallet_id uuid NOT NULL REFERENCES wallets(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    start_intent_id uuid NOT NULL UNIQUE REFERENCES charging_start_intents(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    amount numeric(14,2) NOT NULL,
    currency char(3) NOT NULL,
    status varchar(32) NOT NULL,
    captured_at timestamptz,
    released_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_wallet_holds_amount CHECK (amount >= 0),
    CONSTRAINT chk_wallet_holds_status CHECK (status IN ('HELD', 'CAPTURED', 'RELEASED', 'RECONCILIATION_REQUIRED'))
);

CREATE TABLE hal_command_records (
    cms_command_id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    kind varchar(8) NOT NULL CHECK (kind IN ('START', 'STOP')),
    start_intent_id uuid UNIQUE REFERENCES charging_start_intents(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    charging_session_id uuid REFERENCES charging_sessions(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    hal_command_id uuid UNIQUE,
    state varchar(32) NOT NULL,
    command_expires_at timestamptz NOT NULL,
    last_error_category varchar(64) NOT NULL DEFAULT '',
    last_error_detail varchar(500) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_hal_command_records_session ON hal_command_records (charging_session_id, created_at DESC);

CREATE TABLE hal_fact_receipts (
    fact_id uuid PRIMARY KEY,
    fact_type varchar(64) NOT NULL,
    digest char(64) NOT NULL,
    occurred_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE hal_charger_mappings (
    cms_charger_id uuid PRIMARY KEY REFERENCES chargers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    charger_ocpp_identity varchar(255) NOT NULL UNIQUE,
    sync_state varchar(32) NOT NULL,
    last_sync_error varchar(500) NOT NULL DEFAULT '',
    last_synchronized_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_hal_charger_mappings_sync_state CHECK (sync_state IN ('PENDING', 'SYNCHRONIZED', 'CONFLICT', 'RECONCILIATION_REQUIRED'))
);

CREATE TABLE hal_charger_runtime (
    cms_charger_id uuid PRIMARY KEY REFERENCES chargers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    connection_state varchar(16) NOT NULL CHECK (connection_state IN ('ONLINE', 'OFFLINE', 'UNKNOWN')),
    connection_generation bigint NOT NULL,
    connection_sequence bigint NOT NULL,
    observed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE hal_connector_runtime (
    cms_connector_id uuid PRIMARY KEY REFERENCES connectors(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cms_charger_id uuid NOT NULL REFERENCES chargers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ocpp_connector_status varchar(32) NOT NULL,
    connector_status_sequence bigint NOT NULL,
    observed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_hal_connector_runtime_charger ON hal_connector_runtime (cms_charger_id, connector_status_sequence DESC);
