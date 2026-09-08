-- Per-operation protocol evidence is diagnostic only.  It reuses the existing
-- immutable trace-event/outbox ingestion path and never affects charger state.
ALTER TABLE charger_operations
    ADD COLUMN trace_id uuid NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX uq_charger_operations_trace_id ON charger_operations(trace_id);

ALTER TABLE charging_traces
    ADD COLUMN cms_charger_operation_id uuid NULL,
    ADD COLUMN hal_charger_operation_id uuid NULL;
CREATE UNIQUE INDEX uq_charging_traces_cms_charger_operation
    ON charging_traces(cms_charger_operation_id) WHERE cms_charger_operation_id IS NOT NULL;
CREATE UNIQUE INDEX uq_charging_traces_hal_charger_operation
    ON charging_traces(hal_charger_operation_id) WHERE hal_charger_operation_id IS NOT NULL;
CREATE INDEX ix_charging_traces_cpo_operation
    ON charging_traces(cpo_id, cms_charger_operation_id)
    WHERE cms_charger_operation_id IS NOT NULL;
