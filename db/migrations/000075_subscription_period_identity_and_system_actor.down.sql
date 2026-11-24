-- Deliberately fail safely after any new per-period events / system history.
-- Reverting those rows to the old uniqueness/actor model would lose truth.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cpo_subscription_lifecycle_events WHERE period_sequence IS NOT NULL)
       OR EXISTS (SELECT 1 FROM cpo_subscriptions WHERE current_period_sequence <> 1)
       OR EXISTS (SELECT 1 FROM cpo_subscription_history WHERE actor_user_id IS NULL)
    THEN
        RAISE EXCEPTION 'Unsafe rollback of 000075: new period/system history exists; use a reviewed forward recovery migration';
    END IF;
END
$$;

DROP INDEX uq_cpo_subscription_system_idempotency;
ALTER TABLE cpo_subscription_history ALTER COLUMN actor_user_id SET NOT NULL;
ALTER TABLE cpo_subscription_lifecycle_events
    DROP CONSTRAINT uq_cpo_subscription_lifecycle_period_kind;
DROP INDEX uq_cpo_subscription_lifecycle_legacy;
ALTER TABLE cpo_subscription_lifecycle_events
    ADD CONSTRAINT uq_cpo_subscription_lifecycle_event UNIQUE (subscription_id, kind);
ALTER TABLE cpo_subscription_lifecycle_events
    DROP CONSTRAINT chk_cpo_subscription_event_period_sequence_positive;
ALTER TABLE cpo_subscription_lifecycle_events DROP COLUMN period_sequence;
ALTER TABLE cpo_subscriptions DROP CONSTRAINT chk_cpo_subscription_period_sequence_positive;
ALTER TABLE cpo_subscriptions DROP COLUMN current_period_sequence;
