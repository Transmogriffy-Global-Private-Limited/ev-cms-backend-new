ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP',
            'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP'
        )
    );

DROP TRIGGER IF EXISTS trg_subscription_plan_entitlements_immutable
    ON subscription_plan_entitlements;
DROP FUNCTION IF EXISTS reject_published_entitlement_mutation();
DROP TRIGGER IF EXISTS trg_subscription_plan_versions_immutable
    ON subscription_plan_versions;
DROP FUNCTION IF EXISTS reject_published_subscription_version_mutation();

DROP TABLE IF EXISTS cpo_entitlement_overrides;
DROP TABLE IF EXISTS cpo_subscription_history;
DROP TABLE IF EXISTS cpo_subscriptions;
DROP TABLE IF EXISTS subscription_plan_entitlements;
DROP TABLE IF EXISTS subscription_plan_versions;
DROP TABLE IF EXISTS subscription_plans;
