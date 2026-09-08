ALTER TABLE charger_operations
    DROP CONSTRAINT charger_operations_kind_check;

ALTER TABLE charger_operations
    ADD CONSTRAINT charger_operations_kind_check
    CHECK (kind IN (
        'RESET',
        'UNLOCK_CONNECTOR',
        'CHANGE_AVAILABILITY',
        'CLEAR_CACHE',
        'CHANGE_CONFIGURATION',
        'TRIGGER_MESSAGE',
        'GET_CONFIGURATION'
    ));
