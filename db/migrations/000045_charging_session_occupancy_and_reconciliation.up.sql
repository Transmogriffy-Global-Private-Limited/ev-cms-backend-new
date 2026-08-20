-- Start intents reserve a connector only before a session materializes. A
-- materialized ACTUALLY_STARTED intent is immutable historical evidence; the
-- session then owns connector occupancy.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM charging_start_intents
        WHERE materialized_session_id IS NULL
          AND status IN ('REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED', 'RECONCILIATION_REQUIRED')
        GROUP BY connector_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot create corrected charging start-intent occupancy index: multiple unmaterialized open intents exist for a connector';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM charging_sessions
        WHERE status IN ('ACTIVE', 'STOP_PENDING', 'RECONCILIATION_REQUIRED')
        GROUP BY connector_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot create charging-session occupancy index: multiple occupancy-owning sessions exist for a connector';
    END IF;
END $$;

DROP INDEX IF EXISTS uq_charging_start_intents_open_connector;
CREATE UNIQUE INDEX uq_charging_start_intents_open_connector
    ON charging_start_intents (connector_id)
    WHERE materialized_session_id IS NULL
      AND status IN ('REQUESTED', 'ACCEPTED_FOR_DELIVERY', 'PROTOCOL_ACKNOWLEDGED', 'RECONCILIATION_REQUIRED');

CREATE UNIQUE INDEX uq_charging_sessions_occupying_connector
    ON charging_sessions (connector_id)
    WHERE status IN ('ACTIVE', 'STOP_PENDING', 'RECONCILIATION_REQUIRED');

ALTER TABLE charging_sessions
    DROP CONSTRAINT chk_charging_sessions_status,
    ADD CONSTRAINT chk_charging_sessions_status CHECK (
        status IN (
            'START_PENDING',
            'ACTIVE',
            'STOP_PENDING',
            'RECONCILIATION_REQUIRED',
            'COMPLETED',
            'FAILED'
        )
    );
