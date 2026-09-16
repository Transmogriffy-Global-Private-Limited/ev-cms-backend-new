-- Quiesce old CMS writers before applying: old PERSISTED did not prove absence.
ALTER TABLE charger_operations
    ADD COLUMN dispatch_charger_identity text,
    ADD COLUMN dispatch_connector_number integer,
    ADD COLUMN dispatch_claim_token uuid,
    ADD COLUMN dispatch_claim_expires_at timestamptz,
    ADD COLUMN delivery_attempted_at timestamptz,
    ADD COLUMN recovery_token uuid,
    ADD COLUMN recovery_after timestamptz NOT NULL DEFAULT now();

ALTER TABLE charger_operations DROP CONSTRAINT charger_operations_state_check;
ALTER TABLE charger_operations ADD CONSTRAINT charger_operations_state_check
    CHECK (state IN ('PERSISTED','DISPATCH_CLAIMED','DELIVERY_ATTEMPTED',
                     'HAL_ACCEPTED','OCPP_CONFIRMED','RECONCILIATION_REQUIRED','CONFIRMED_ABSENT'));
-- Do not invent historical mapping or an exact historical attempt timestamp.
UPDATE charger_operations SET state='RECONCILIATION_REQUIRED',
    failure_category='legacy_delivery_unknown', completed_at=NULL, updated_at=now()
    WHERE state='PERSISTED';
UPDATE charger_operations SET completed_at=NULL
    WHERE state IN ('RECONCILIATION_REQUIRED','HAL_ACCEPTED');

ALTER TABLE charger_operations ADD CONSTRAINT charger_operations_dispatch_check CHECK (
    (state NOT IN ('PERSISTED','DISPATCH_CLAIMED') OR
        (delivery_attempted_at IS NULL AND dispatch_charger_identity IS NOT NULL
         AND length(dispatch_charger_identity)>0 AND dispatch_connector_number IS NOT NULL
         AND dispatch_connector_number>=0))
    AND (state <> 'DELIVERY_ATTEMPTED' OR delivery_attempted_at IS NOT NULL)
    AND ((state='DISPATCH_CLAIMED' AND dispatch_claim_token IS NOT NULL AND dispatch_claim_expires_at IS NOT NULL)
         OR (state<>'DISPATCH_CLAIMED' AND dispatch_claim_token IS NULL AND dispatch_claim_expires_at IS NULL))
    AND (state NOT IN ('PERSISTED','DISPATCH_CLAIMED','DELIVERY_ATTEMPTED','HAL_ACCEPTED','RECONCILIATION_REQUIRED') OR completed_at IS NULL)
);
CREATE INDEX ix_charger_operations_recovery ON charger_operations(recovery_after,id)
    WHERE state IN ('PERSISTED','DISPATCH_CLAIMED','DELIVERY_ATTEMPTED','HAL_ACCEPTED','RECONCILIATION_REQUIRED');

-- The original destination and request are audit truth, not live inventory.
-- Never erase evidence that the external boundary may have been crossed.
CREATE FUNCTION guard_charger_operation_dispatch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.trace_id,NEW.cpo_id,NEW.charger_id,NEW.connector_id,
           NEW.kind,NEW.parameters,NEW.correlation_id,NEW.idempotency_key,NEW.request_digest,
           NEW.dispatch_charger_identity,NEW.dispatch_connector_number)
       IS DISTINCT FROM
       ROW(OLD.id,OLD.trace_id,OLD.cpo_id,OLD.charger_id,OLD.connector_id,
           OLD.kind,OLD.parameters,OLD.correlation_id,OLD.idempotency_key,OLD.request_digest,
           OLD.dispatch_charger_identity,OLD.dispatch_connector_number) THEN
        RAISE EXCEPTION 'charger operation dispatch inputs are immutable' USING ERRCODE='23514';
    END IF;
    IF OLD.delivery_attempted_at IS NOT NULL AND NEW.delivery_attempted_at IS DISTINCT FROM OLD.delivery_attempted_at THEN
        RAISE EXCEPTION 'charger operation delivery evidence is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.state IN ('PERSISTED','DISPATCH_CLAIMED') AND OLD.state NOT IN ('PERSISTED','DISPATCH_CLAIMED') THEN
        RAISE EXCEPTION 'charger operation cannot return to dispatch queue' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER charger_operation_dispatch_guard BEFORE UPDATE ON charger_operations
    FOR EACH ROW EXECUTE FUNCTION guard_charger_operation_dispatch();
