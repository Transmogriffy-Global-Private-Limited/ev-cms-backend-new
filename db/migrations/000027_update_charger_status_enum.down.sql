ALTER TABLE chargers DROP CONSTRAINT chk_chargers_status;
ALTER TABLE connectors DROP CONSTRAINT chk_connectors_status;

ALTER TABLE chargers ALTER COLUMN status SET DEFAULT 'OFFLINE';
ALTER TABLE connectors ALTER COLUMN status SET DEFAULT 'AVAILABLE';

UPDATE chargers SET status = 'AVAILABLE' WHERE status = 'ACTIVE';
UPDATE chargers SET status = 'OFFLINE' WHERE status = 'INACTIVE';
UPDATE chargers SET status = 'SUSPENDED_EVSE' WHERE status = 'SUSPENDED';
UPDATE chargers SET status = 'UNAVAILABLE' WHERE status = 'UNDERMAINTENANCE';
UPDATE chargers SET status = 'UNAVAILABLE' WHERE status = 'DECOMMISSIONED';

UPDATE connectors SET status = 'AVAILABLE' WHERE status = 'ACTIVE';
UPDATE connectors SET status = 'OFFLINE' WHERE status = 'INACTIVE';
UPDATE connectors SET status = 'SUSPENDED_EVSE' WHERE status = 'SUSPENDED';
UPDATE connectors SET status = 'UNAVAILABLE' WHERE status = 'UNDERMAINTENANCE';
UPDATE connectors SET status = 'UNAVAILABLE' WHERE status = 'DECOMMISSIONED';

ALTER TABLE chargers ADD CONSTRAINT chk_chargers_status CHECK (
    status IN (
        'AVAILABLE',
        'PREPARING',
        'CHARGING',
        'SUSPENDED_EV',
        'SUSPENDED_EVSE',
        'FINISHING',
        'RESERVED',
        'UNAVAILABLE',
        'FAULTED',
        'OFFLINE'
    )
);

ALTER TABLE connectors ADD CONSTRAINT chk_connectors_status CHECK (
    status IN (
        'AVAILABLE',
        'PREPARING',
        'CHARGING',
        'SUSPENDED_EV',
        'SUSPENDED_EVSE',
        'FINISHING',
        'RESERVED',
        'UNAVAILABLE',
        'FAULTED',
        'OFFLINE'
    )
);
