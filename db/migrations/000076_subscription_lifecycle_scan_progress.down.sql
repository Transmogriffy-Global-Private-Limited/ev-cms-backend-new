-- Deleting the cursor only loses scheduling progress (events remain durable).
DROP INDEX IF EXISTS ix_cpo_subscriptions_lifecycle_cursor;
DROP TABLE IF EXISTS subscription_lifecycle_scan_progress;
