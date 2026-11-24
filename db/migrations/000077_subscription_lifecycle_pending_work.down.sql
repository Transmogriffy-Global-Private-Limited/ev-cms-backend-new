-- Never discard unfinished durable work during a rollback.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM subscription_lifecycle_pending_work) THEN
        RAISE EXCEPTION 'Unsafe rollback of 000077: pending lifecycle work exists; drain or reconcile it first';
    END IF;
END
$$;
DROP INDEX ix_subscription_lifecycle_pending_due;
DROP TABLE subscription_lifecycle_pending_work;
