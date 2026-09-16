-- Rolling back to an unfenced dispatcher is unsafe while any operations exist.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM charger_operations) THEN
        RAISE EXCEPTION 'cannot remove dispatch safety while charger operations exist';
    END IF;
END $$;
DROP TRIGGER charger_operation_dispatch_guard ON charger_operations;
DROP FUNCTION guard_charger_operation_dispatch();
DROP INDEX ix_charger_operations_recovery;
ALTER TABLE charger_operations DROP CONSTRAINT charger_operations_dispatch_check;
ALTER TABLE charger_operations DROP CONSTRAINT charger_operations_state_check;
ALTER TABLE charger_operations ADD CONSTRAINT charger_operations_state_check
    CHECK (state IN ('PERSISTED','HAL_ACCEPTED','OCPP_CONFIRMED','RECONCILIATION_REQUIRED','CONFIRMED_ABSENT'));
ALTER TABLE charger_operations
    DROP COLUMN dispatch_charger_identity, DROP COLUMN dispatch_connector_number,
    DROP COLUMN dispatch_claim_token, DROP COLUMN dispatch_claim_expires_at,
    DROP COLUMN delivery_attempted_at, DROP COLUMN recovery_token, DROP COLUMN recovery_after;
