DROP TABLE IF EXISTS charging_trace_events;
DROP INDEX IF EXISTS ix_hal_command_records_trace_id;
DROP INDEX IF EXISTS uq_charging_sessions_trace_id;
DROP INDEX IF EXISTS uq_charging_start_intents_trace_id;
ALTER TABLE hal_command_records DROP COLUMN IF EXISTS trace_id;
ALTER TABLE charging_sessions DROP COLUMN IF EXISTS trace_id;
ALTER TABLE charging_start_intents DROP COLUMN IF EXISTS trace_id;
