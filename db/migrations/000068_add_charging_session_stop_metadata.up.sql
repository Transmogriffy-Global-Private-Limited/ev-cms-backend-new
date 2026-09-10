ALTER TABLE charging_sessions
    ADD COLUMN requested_stop_initiator varchar(50),
    ADD COLUMN requested_stop_reason varchar(100),
    ADD COLUMN ocpp_stop_reason varchar(50);

-- Existing stop_reason has ambiguous historical provenance. Do not invent
-- canonical requested/OCPP provenance by backfilling these new columns.
