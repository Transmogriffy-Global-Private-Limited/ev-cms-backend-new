DROP INDEX IF EXISTS ix_charging_traces_cpo_operation;
DROP INDEX IF EXISTS uq_charging_traces_hal_charger_operation;
DROP INDEX IF EXISTS uq_charging_traces_cms_charger_operation;
ALTER TABLE charging_traces DROP COLUMN IF EXISTS hal_charger_operation_id;
ALTER TABLE charging_traces DROP COLUMN IF EXISTS cms_charger_operation_id;
DROP INDEX IF EXISTS uq_charger_operations_trace_id;
ALTER TABLE charger_operations DROP COLUMN IF EXISTS trace_id;
