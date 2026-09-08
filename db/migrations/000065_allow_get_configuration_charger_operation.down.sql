DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM charger_operations
        WHERE kind = 'GET_CONFIGURATION'
    ) THEN
        RAISE EXCEPTION 'cannot rollback charger operation kind constraint while GET_CONFIGURATION rows exist';
    END IF;
END
$$;

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
        'TRIGGER_MESSAGE'
    ));
