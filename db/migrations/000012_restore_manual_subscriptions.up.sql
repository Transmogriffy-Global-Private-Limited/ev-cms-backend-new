-- Restore only the subscription catalog and CPO entitlement records retired by
-- migration 000009. Platform billing remains in retired_commercial and no
-- lifecycle worker is re-enabled: every provider-like state transition is a
-- deliberate platform-superadmin command.
ALTER TABLE retired_commercial.subscription_plans SET SCHEMA public;
ALTER TABLE retired_commercial.subscription_plan_versions SET SCHEMA public;
ALTER TABLE retired_commercial.subscription_plan_entitlements SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_subscriptions SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_subscription_history SET SCHEMA public;
ALTER TABLE retired_commercial.cpo_entitlement_overrides SET SCHEMA public;

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

UPDATE worker_instances
   SET required = FALSE,
       reported_status = 'DISABLED',
       metadata = (metadata - 'retired_reason') || jsonb_build_object(
           'disabled_reason',
           'manual_subscription_management_has_no_automatic_lifecycle'
       ),
       updated_at = now()
 WHERE worker_name IN ('subscription-lifecycle', 'billing-maintenance');
