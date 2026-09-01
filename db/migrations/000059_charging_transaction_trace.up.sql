ALTER TABLE charging_start_intents ADD COLUMN trace_id uuid NULL;
ALTER TABLE charging_sessions ADD COLUMN trace_id uuid NULL;
ALTER TABLE hal_command_records ADD COLUMN trace_id uuid NULL;

CREATE UNIQUE INDEX uq_charging_start_intents_trace_id ON charging_start_intents(trace_id) WHERE trace_id IS NOT NULL;
CREATE UNIQUE INDEX uq_charging_sessions_trace_id ON charging_sessions(trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX ix_hal_command_records_trace_id ON hal_command_records(trace_id);

CREATE TABLE charging_trace_events (
    id uuid PRIMARY KEY,
    trace_id uuid NOT NULL,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON DELETE RESTRICT,
    session_id uuid NULL REFERENCES charging_sessions(id) ON DELETE SET NULL,
    source varchar(32) NOT NULL,
    target varchar(32) NOT NULL,
    category varchar(48) NOT NULL,
    protocol varchar(24) NOT NULL,
    phase varchar(24) NOT NULL,
    summary varchar(200) NOT NULL,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL,
    state_before varchar(64) NOT NULL DEFAULT '',
    state_after varchar(64) NOT NULL DEFAULT '',
    correlation_id varchar(128) NOT NULL DEFAULT '',
    data jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ix_charging_trace_events_cursor ON charging_trace_events(trace_id, occurred_at DESC, id DESC);
