DROP TABLE IF EXISTS hal_connector_runtime;
DROP TABLE IF EXISTS hal_charger_runtime;
DROP TABLE IF EXISTS hal_charger_mappings;
DROP TABLE IF EXISTS hal_fact_receipts;
DROP TABLE IF EXISTS hal_command_records;
DROP TABLE IF EXISTS wallet_holds;
DROP INDEX IF EXISTS uq_charging_start_intents_open_connector;
DROP TABLE IF EXISTS charging_start_intents;

DROP INDEX IF EXISTS uq_charging_sessions_hal_transaction;
DROP INDEX IF EXISTS uq_charging_sessions_start_intent;
ALTER TABLE charging_sessions
    DROP COLUMN IF EXISTS settlement_status,
    DROP COLUMN IF EXISTS meter_sequence,
    DROP COLUMN IF EXISTS meter_observed_at,
    DROP COLUMN IF EXISTS latest_meter_wh,
    DROP COLUMN IF EXISTS hal_transaction_id,
    DROP COLUMN IF EXISTS start_intent_id;
ALTER TABLE charging_sessions
    ALTER COLUMN transaction_id TYPE integer;
