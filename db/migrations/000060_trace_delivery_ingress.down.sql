DROP TABLE IF EXISTS charging_traces;
DROP INDEX IF EXISTS ix_charging_trace_events_replay;
DROP INDEX IF EXISTS uq_charging_trace_events_ingestion_sequence;
ALTER TABLE charging_trace_events DROP COLUMN IF EXISTS ingestion_sequence;
DROP SEQUENCE IF EXISTS charging_trace_events_ingestion_sequence_seq;
ALTER TABLE charging_trace_events DROP COLUMN IF EXISTS immutable_content_sha256;
