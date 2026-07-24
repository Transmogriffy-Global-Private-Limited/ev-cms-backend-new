ALTER TABLE retired_commercial.subscription_plans
    SET SCHEMA public;
ALTER TABLE retired_commercial.subscription_plan_versions
    SET SCHEMA public;
ALTER TABLE retired_commercial.subscription_plan_entitlements
    SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_subscriptions
    SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_subscription_history
    SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_entitlement_overrides
    SET SCHEMA public;

ALTER TABLE retired_commercial.cpo_billing_accounts
    SET SCHEMA public;
ALTER TABLE retired_commercial.platform_invoices
    SET SCHEMA public;
ALTER TABLE retired_commercial.platform_invoice_lines
    SET SCHEMA public;
ALTER TABLE retired_commercial.platform_payments
    SET SCHEMA public;
ALTER TABLE retired_commercial.platform_payment_allocations
    SET SCHEMA public;

CREATE FUNCTION reject_published_subscription_version_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status = 'PUBLISHED' THEN
        RAISE EXCEPTION 'published subscription plan versions are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_subscription_plan_versions_immutable
BEFORE UPDATE OR DELETE ON subscription_plan_versions
FOR EACH ROW
EXECUTE FUNCTION reject_published_subscription_version_mutation();

CREATE FUNCTION reject_published_entitlement_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    version_status varchar(20);
BEGIN
    SELECT status
      INTO version_status
      FROM subscription_plan_versions
     WHERE id = COALESCE(OLD.plan_version_id, NEW.plan_version_id);
    IF version_status = 'PUBLISHED' THEN
        RAISE EXCEPTION 'published subscription plan entitlements are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_subscription_plan_entitlements_immutable
BEFORE UPDATE OR DELETE ON subscription_plan_entitlements
FOR EACH ROW
EXECUTE FUNCTION reject_published_entitlement_mutation();

CREATE FUNCTION protect_issued_platform_invoice()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status <> 'DRAFT' AND (
        NEW.invoice_number IS DISTINCT FROM OLD.invoice_number
        OR NEW.cpo_id IS DISTINCT FROM OLD.cpo_id
        OR NEW.billing_account_id IS DISTINCT FROM OLD.billing_account_id
        OR NEW.subscription_id IS DISTINCT FROM OLD.subscription_id
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.subtotal_minor IS DISTINCT FROM OLD.subtotal_minor
        OR NEW.tax_minor IS DISTINCT FROM OLD.tax_minor
        OR NEW.total_minor IS DISTINCT FROM OLD.total_minor
        OR NEW.period_starts_at IS DISTINCT FROM OLD.period_starts_at
        OR NEW.period_ends_at IS DISTINCT FROM OLD.period_ends_at
        OR NEW.issued_at IS DISTINCT FROM OLD.issued_at
        OR NEW.due_at IS DISTINCT FROM OLD.due_at
        OR NEW.external_reference IS DISTINCT FROM OLD.external_reference
    ) THEN
        RAISE EXCEPTION 'issued platform invoice commercial terms are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_platform_invoices_protect_issued
BEFORE UPDATE ON platform_invoices
FOR EACH ROW
EXECUTE FUNCTION protect_issued_platform_invoice();

CREATE FUNCTION protect_platform_invoice_line()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    invoice_status varchar(30);
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT status
          INTO invoice_status
          FROM platform_invoices
         WHERE id = OLD.invoice_id;
    ELSE
        SELECT status
          INTO invoice_status
          FROM platform_invoices
         WHERE id = NEW.invoice_id;
    END IF;
    IF invoice_status <> 'DRAFT' THEN
        RAISE EXCEPTION 'issued platform invoice lines are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_platform_invoice_lines_protect_issued
BEFORE UPDATE OR DELETE ON platform_invoice_lines
FOR EACH ROW
EXECUTE FUNCTION protect_platform_invoice_line();

UPDATE worker_instances
   SET required = TRUE,
       reported_status = 'HEALTHY',
       last_heartbeat_at = now(),
       metadata = metadata - 'retired_reason',
       updated_at = now()
 WHERE worker_name IN ('subscription-lifecycle', 'billing-maintenance');

DROP SCHEMA retired_commercial;
