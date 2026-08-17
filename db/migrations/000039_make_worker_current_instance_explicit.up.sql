-- Worker rows are durable process history. Exactly one row per logical worker
-- is the authoritative current operational projection in this single-instance
-- deployment; restarted incarnations supersede, rather than duplicate, it.
ALTER TABLE worker_instances
    ADD COLUMN is_current boolean NOT NULL DEFAULT false;

WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY worker_name
               ORDER BY last_heartbeat_at DESC, started_at DESC, id DESC
           ) AS position
      FROM worker_instances
)
UPDATE worker_instances AS worker
   SET is_current = true
  FROM ranked
 WHERE ranked.id = worker.id
   AND ranked.position = 1;

CREATE UNIQUE INDEX uq_worker_instances_current_worker
    ON worker_instances (worker_name)
    WHERE is_current = true;
