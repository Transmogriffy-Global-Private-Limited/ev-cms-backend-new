-- Scheduler-only state for fair bounded exact-HAL transaction reconciliation.
-- It deliberately does not alter charging-session business timestamps or state.
CREATE TABLE charging_reconciliation_cursors (
    name varchar(64) PRIMARY KEY,
    last_session_id uuid NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_charging_sessions_open_hal_reconciliation_cursor
    ON charging_sessions (id)
    WHERE end_time IS NULL
      AND hal_transaction_id IS NOT NULL
      AND status IN ('ACTIVE', 'STOP_PENDING', 'RECONCILIATION_REQUIRED');
