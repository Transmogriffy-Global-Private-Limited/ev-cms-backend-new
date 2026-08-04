-- Migration 16 introduced independent charger inventory. Some development
-- databases subsequently applied and then removed follow-up migrations that
-- restored hub_id NOT NULL; reconcile those databases to the current contract.
ALTER TABLE chargers
    ALTER COLUMN hub_id DROP NOT NULL;
