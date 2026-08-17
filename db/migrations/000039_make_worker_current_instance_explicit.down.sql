DROP INDEX IF EXISTS uq_worker_instances_current_worker;

ALTER TABLE worker_instances
    DROP COLUMN IF EXISTS is_current;
