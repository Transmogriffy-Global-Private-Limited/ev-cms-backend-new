-- Subscription expiry must be unique PER PERIOD, not once for the life of
-- a subscription. Legacy lifecycle rows retain NULL period identity rather
-- than being silently assigned to an invented historical period.
ALTER TABLE cpo_subscriptions
    ADD COLUMN current_period_sequence bigint NOT NULL DEFAULT 1;
ALTER TABLE cpo_subscriptions
    ADD CONSTRAINT chk_cpo_subscription_period_sequence_positive
    CHECK (current_period_sequence > 0);

ALTER TABLE cpo_subscription_lifecycle_events
    ADD COLUMN period_sequence bigint;
ALTER TABLE cpo_subscription_lifecycle_events
    ADD CONSTRAINT chk_cpo_subscription_event_period_sequence_positive
    CHECK (period_sequence IS NULL OR period_sequence > 0);

ALTER TABLE cpo_subscription_lifecycle_events
    DROP CONSTRAINT uq_cpo_subscription_lifecycle_event;

-- Preserve legacy unversioned deduplication without modifying any old event.
CREATE UNIQUE INDEX uq_cpo_subscription_lifecycle_legacy
    ON cpo_subscription_lifecycle_events (subscription_id, kind)
    WHERE period_sequence IS NULL;
ALTER TABLE cpo_subscription_lifecycle_events
    ADD CONSTRAINT uq_cpo_subscription_lifecycle_period_kind
    UNIQUE (subscription_id, period_sequence, kind);

-- Automatic expiry has a SYSTEM actor, not a fabricated human users(id).
-- The existing FK still validates all non-NULL human actor IDs.
ALTER TABLE cpo_subscription_history
    ALTER COLUMN actor_user_id DROP NOT NULL;
CREATE UNIQUE INDEX uq_cpo_subscription_system_idempotency
    ON cpo_subscription_history (subscription_id, idempotency_key)
    WHERE actor_user_id IS NULL;
