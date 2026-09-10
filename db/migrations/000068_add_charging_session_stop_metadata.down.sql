ALTER TABLE charging_sessions
    DROP COLUMN IF EXISTS ocpp_stop_reason,
    DROP COLUMN IF EXISTS requested_stop_reason,
    DROP COLUMN IF EXISTS requested_stop_initiator;
