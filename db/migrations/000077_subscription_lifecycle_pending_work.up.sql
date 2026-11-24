-- A durable work row is inserted BEFORE advancing the scanner cursor.
-- Leases survive worker death and retries are independently scheduled;
-- subscription transitions/mail remain guarded by existing period uniqueness.
CREATE TABLE subscription_lifecycle_pending_work (
    subscription_id uuid PRIMARY KEY REFERENCES cpo_subscriptions(id) ON DELETE CASCADE,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    claim_token uuid,
    attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- Upgrade recovery: previously skipped entries may lie behind an existing
-- cursor. Seed all currently eligible rows once, without inventing an event or
-- touching subscription state. New discovery stays bounded thereafter.
INSERT INTO subscription_lifecycle_pending_work
    (subscription_id, next_attempt_at, created_at, updated_at)
SELECT id, now(), now(), now()
FROM cpo_subscriptions
WHERE status IN ('TRIAL', 'ACTIVE', 'PAUSED', 'PAST_DUE')
  AND current_period_ends_at <= now() + interval '7 days'
ON CONFLICT (subscription_id) DO NOTHING;

CREATE INDEX ix_subscription_lifecycle_pending_due
    ON subscription_lifecycle_pending_work (next_attempt_at, subscription_id);
