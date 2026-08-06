ALTER TABLE chargers DROP CONSTRAINT chk_chargers_status;
ALTER TABLE connectors DROP CONSTRAINT chk_connectors_status;

ALTER TABLE chargers ALTER COLUMN status SET DEFAULT 'INACTIVE';
ALTER TABLE connectors ALTER COLUMN status SET DEFAULT 'INACTIVE';

UPDATE chargers SET status = 'INACTIVE' WHERE status = 'OFFLINE';
UPDATE connectors SET status = 'INACTIVE' WHERE status = 'AVAILABLE';

ALTER TABLE chargers ADD CONSTRAINT chk_chargers_status CHECK (
    status IN (
        'ACTIVE',
        'INACTIVE',
        'SUSPENDED',
        'UNDERMAINTENANCE',
        'DECOMMISSIONED'
    )
);
ALTER TABLE connectors ADD CONSTRAINT chk_connectors_status CHECK (
    status IN (
        'ACTIVE',
        'INACTIVE',
        'SUSPENDED',
        'UNDERMAINTENANCE',
        'DECOMMISSIONED'
    )
);
