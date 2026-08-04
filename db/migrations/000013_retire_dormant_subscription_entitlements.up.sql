-- Feature-level enforcement has not been defined. Preserve the prematurely
-- restored entitlement data, but remove it from the active subscription
-- runtime until a concrete module catalog and server-side gates are approved.
ALTER TABLE cpo_entitlement_overrides
    SET SCHEMA retired_commercial;
ALTER TABLE subscription_plan_entitlements
    SET SCHEMA retired_commercial;
