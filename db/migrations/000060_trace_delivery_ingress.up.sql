-- CMS owns the CPO-visible diagnostic trace history. This schema is additive
-- and does not modify authoritative charging, command, wallet, or fact rows.
CREATE TABLE charging_traces (
    trace_id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON DELETE RESTRICT,
    cms_start_intent_id uuid NULL,
    cms_charging_session_id uuid NULL,
    cms_command_id uuid NULL,
    hal_transaction_id uuid NULL,
    ocpp_transaction_id bigint NULL,
    charger_ocpp_identity varchar(255) NOT NULL DEFAULT '',
    ocpp_connector_number integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (ocpp_connector_number >= 0),
    CHECK (ocpp_transaction_id IS NULL OR ocpp_transaction_id > 0)
);
CREATE INDEX ix_charging_traces_cpo_trace ON charging_traces(cpo_id, trace_id);
CREATE UNIQUE INDEX uq_charging_traces_start_intent ON charging_traces(cms_start_intent_id) WHERE cms_start_intent_id IS NOT NULL;
CREATE UNIQUE INDEX uq_charging_traces_session ON charging_traces(cms_charging_session_id) WHERE cms_charging_session_id IS NOT NULL;
CREATE UNIQUE INDEX uq_charging_traces_hal_transaction ON charging_traces(hal_transaction_id) WHERE hal_transaction_id IS NOT NULL;

ALTER TABLE charging_trace_events ADD COLUMN immutable_content_sha256 varchar(64) NOT NULL DEFAULT '';
CREATE SEQUENCE charging_trace_events_ingestion_sequence_seq;
ALTER TABLE charging_trace_events ADD COLUMN ingestion_sequence bigint;
ALTER TABLE charging_trace_events ALTER COLUMN ingestion_sequence SET DEFAULT nextval('charging_trace_events_ingestion_sequence_seq');
UPDATE charging_trace_events SET ingestion_sequence = nextval('charging_trace_events_ingestion_sequence_seq') WHERE ingestion_sequence IS NULL;
ALTER TABLE charging_trace_events ALTER COLUMN ingestion_sequence SET NOT NULL;
ALTER SEQUENCE charging_trace_events_ingestion_sequence_seq OWNED BY charging_trace_events.ingestion_sequence;
CREATE UNIQUE INDEX uq_charging_trace_events_ingestion_sequence ON charging_trace_events(ingestion_sequence);
CREATE INDEX ix_charging_trace_events_replay ON charging_trace_events(trace_id, ingestion_sequence ASC);

INSERT INTO charging_traces(trace_id,cpo_id,cms_start_intent_id,cms_charging_session_id,cms_command_id,created_at,updated_at)
SELECT trace_id,cpo_id,NULL,NULL,NULL,MIN(recorded_at),MAX(recorded_at)
FROM charging_trace_events GROUP BY trace_id,cpo_id
ON CONFLICT (trace_id) DO NOTHING;
