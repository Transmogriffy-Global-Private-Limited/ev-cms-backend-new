DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM mail_outbox
         WHERE template IN (
             'CPO_SUBSCRIPTION_CHANGED',
             'CPO_PLATFORM_INVOICE_ISSUED'
         )
           AND status IN ('PENDING', 'PROCESSING')
    ) THEN
        RAISE EXCEPTION
            'cannot retire subscriptions and platform billing while related mail is pending';
    END IF;
END;
$$;

CREATE SCHEMA retired_commercial;

DROP TRIGGER IF EXISTS trg_platform_invoice_lines_protect_issued
    ON platform_invoice_lines;
DROP FUNCTION IF EXISTS protect_platform_invoice_line();
DROP TRIGGER IF EXISTS trg_platform_invoices_protect_issued
    ON platform_invoices;
DROP FUNCTION IF EXISTS protect_issued_platform_invoice();
DROP TRIGGER IF EXISTS trg_subscription_plan_entitlements_immutable
    ON subscription_plan_entitlements;
DROP FUNCTION IF EXISTS reject_published_entitlement_mutation();
DROP TRIGGER IF EXISTS trg_subscription_plan_versions_immutable
    ON subscription_plan_versions;
DROP FUNCTION IF EXISTS reject_published_subscription_version_mutation();

ALTER TABLE platform_payment_allocations
    SET SCHEMA retired_commercial;
ALTER TABLE platform_invoice_lines
    SET SCHEMA retired_commercial;
ALTER TABLE platform_payments
    SET SCHEMA retired_commercial;
ALTER TABLE platform_invoices
    SET SCHEMA retired_commercial;
ALTER TABLE cpo_billing_accounts
    SET SCHEMA retired_commercial;

ALTER TABLE cpo_entitlement_overrides
    SET SCHEMA retired_commercial;
ALTER TABLE cpo_subscription_history
    SET SCHEMA retired_commercial;
ALTER TABLE cpo_subscriptions
    SET SCHEMA retired_commercial;
ALTER TABLE subscription_plan_entitlements
    SET SCHEMA retired_commercial;
ALTER TABLE subscription_plan_versions
    SET SCHEMA retired_commercial;
ALTER TABLE subscription_plans
    SET SCHEMA retired_commercial;

UPDATE worker_instances
   SET required = FALSE,
       reported_status = 'DISABLED',
       metadata = metadata || jsonb_build_object(
           'retired_reason',
           'subscriptions_and_platform_billing_removed'
       ),
       updated_at = now()
 WHERE worker_name IN ('subscription-lifecycle', 'billing-maintenance');
