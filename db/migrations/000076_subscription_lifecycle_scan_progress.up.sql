-- Durable scanner cursor. Bounded batches advance across runs, surviving restarts;
-- failure of one CPO cannot indefinitely starve the following CPOs.
CREATE TABLE subscription_lifecycle_scan_progress (
    worker_name varchar(60) PRIMARY KEY,
    cursor_end_at timestamptz,
    cursor_id uuid,
    last_scan_at timestamptz,
    processed_total bigint NOT NULL DEFAULT 0,
    failures_total bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_subscription_lifecycle_scan_cursor_pair CHECK ((cursor_end_at IS NULL) = (cursor_id IS NULL)),
    CONSTRAINT chk_subscription_lifecycle_scan_counters CHECK (processed_total >= 0 AND failures_total >= 0)
);
INSERT INTO subscription_lifecycle_scan_progress (worker_name) VALUES ('subscription-lifecycle');

-- Enables keyset traversal of future-due and overdue active subscriptions.
CREATE INDEX ix_cpo_subscriptions_lifecycle_cursor
    ON cpo_subscriptions (current_period_ends_at, id)
    WHERE status IN ('TRIAL','ACTIVE','PAUSED','PAST_DUE');
