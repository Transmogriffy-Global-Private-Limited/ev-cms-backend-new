-- Reversal is intentionally guarded: it must not discard reconciliation
-- truth or restore the former unsafe ACTUALLY_STARTED reservation predicate.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM charging_sessions WHERE status = 'RECONCILIATION_REQUIRED') THEN
        RAISE EXCEPTION 'cannot reverse charging-session reconciliation migration while reconciliation-required sessions exist';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM charging_start_intents
        WHERE status IN ('REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED', 'ACTUALLY_STARTED', 'RECONCILIATION_REQUIRED')
        GROUP BY connector_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot restore former start-intent index: historical ACTUALLY_STARTED intents would conflict';
    END IF;
END $$;

DROP INDEX IF EXISTS uq_charging_sessions_occupying_connector;
DROP INDEX IF EXISTS uq_charging_start_intents_open_connector;
CREATE UNIQUE INDEX uq_charging_start_intents_open_connector
    ON charging_start_intents (connector_id)
    WHERE status IN ('REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED', 'ACTUALLY_STARTED', 'RECONCILIATION_REQUIRED');

ALTER TABLE charging_sessions
    DROP CONSTRAINT chk_charging_sessions_status,
    ADD CONSTRAINT chk_charging_sessions_status CHECK (
        status IN ('START_PENDING', 'ACTIVE', 'STOP_PENDING', 'COMPLETED', 'FAILED')
    );
