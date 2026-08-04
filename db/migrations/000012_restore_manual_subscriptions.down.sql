DROP TRIGGER IF EXISTS trg_subscription_plan_entitlements_immutable
    ON subscription_plan_entitlements;
DROP FUNCTION IF EXISTS reject_published_entitlement_mutation();
DROP TRIGGER IF EXISTS trg_subscription_plan_versions_immutable
    ON subscription_plan_versions;
DROP FUNCTION IF EXISTS reject_published_subscription_version_mutation();

ALTER TABLE cpo_entitlement_overrides SET SCHEMA retired_commercial;
ALTER TABLE cpo_subscription_history SET SCHEMA retired_commercial;
ALTER TABLE cpo_subscriptions SET SCHEMA retired_commercial;
ALTER TABLE subscription_plan_entitlements SET SCHEMA retired_commercial;
ALTER TABLE subscription_plan_versions SET SCHEMA retired_commercial;
ALTER TABLE subscription_plans SET SCHEMA retired_commercial;

UPDATE worker_instances
   SET required = FALSE,
       reported_status = 'DISABLED',
       metadata = (metadata - 'disabled_reason') || jsonb_build_object(
           'retired_reason',
           'subscriptions_and_platform_billing_removed'
       ),
       updated_at = now()
 WHERE worker_name IN ('subscription-lifecycle', 'billing-maintenance');
