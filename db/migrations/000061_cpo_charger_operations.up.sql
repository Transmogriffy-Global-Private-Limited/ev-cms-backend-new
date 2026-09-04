-- CPO control/audit records are separate from charging Start/Stop commands.
CREATE TABLE charger_operations (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON DELETE RESTRICT,
    charger_id uuid NOT NULL REFERENCES chargers(id) ON DELETE RESTRICT,
    connector_id uuid NULL REFERENCES connectors(id) ON DELETE RESTRICT,
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key varchar(128) NOT NULL,
    request_digest char(64) NOT NULL,
    correlation_id varchar(64) NOT NULL,
    kind varchar(32) NOT NULL CHECK (kind IN ('RESET','UNLOCK_CONNECTOR','CHANGE_AVAILABILITY','CLEAR_CACHE','CHANGE_CONFIGURATION','TRIGGER_MESSAGE')),
    parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    hal_operation_id uuid NULL UNIQUE,
    state varchar(32) NOT NULL CHECK (state IN ('PERSISTED','HAL_ACCEPTED','OCPP_CONFIRMED','RECONCILIATION_REQUIRED','CONFIRMED_ABSENT')),
    ocpp_result varchar(64) NOT NULL DEFAULT '',
    failure_category varchar(64) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    completed_at timestamptz NULL,
    UNIQUE (cpo_id, idempotency_key)
);
CREATE INDEX ix_charger_operations_cpo_charger_created ON charger_operations(cpo_id, charger_id, created_at DESC);
CREATE INDEX ix_charger_operations_reconciliation ON charger_operations(state, updated_at) WHERE state='RECONCILIATION_REQUIRED';
