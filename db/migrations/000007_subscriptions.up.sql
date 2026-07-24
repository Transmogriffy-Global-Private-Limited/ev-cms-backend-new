CREATE TABLE subscription_plans (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(80) NOT NULL UNIQUE,
    name varchar(150) NOT NULL,
    description varchar(2000) NOT NULL DEFAULT '',
    status varchar(20) NOT NULL DEFAULT 'DRAFT',
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_subscription_plans_code
        CHECK (code ~ '^[a-z0-9]+(?:_[a-z0-9]+)*$'),
    CONSTRAINT chk_subscription_plans_name
        CHECK (btrim(name) <> ''),
    CONSTRAINT chk_subscription_plans_status
        CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED'))
);

CREATE TABLE subscription_plan_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id uuid NOT NULL
        REFERENCES subscription_plans(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    version integer NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'DRAFT',
    currency char(3) NOT NULL,
    price_minor bigint NOT NULL,
    billing_interval varchar(20) NOT NULL,
    interval_count integer NOT NULL DEFAULT 1,
    trial_days integer NOT NULL DEFAULT 0,
    published_at timestamptz,
    published_by uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_subscription_plan_versions UNIQUE (plan_id, version),
    CONSTRAINT chk_subscription_plan_versions_version CHECK (version > 0),
    CONSTRAINT chk_subscription_plan_versions_status
        CHECK (status IN ('DRAFT', 'PUBLISHED')),
    CONSTRAINT chk_subscription_plan_versions_currency
        CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_subscription_plan_versions_price CHECK (price_minor >= 0),
    CONSTRAINT chk_subscription_plan_versions_interval
        CHECK (billing_interval IN ('MONTHLY', 'YEARLY')),
    CONSTRAINT chk_subscription_plan_versions_interval_count
        CHECK (interval_count BETWEEN 1 AND 120),
    CONSTRAINT chk_subscription_plan_versions_trial
        CHECK (trial_days BETWEEN 0 AND 365),
    CONSTRAINT chk_subscription_plan_versions_publication CHECK (
        (status = 'DRAFT' AND published_at IS NULL AND published_by IS NULL)
        OR
        (status = 'PUBLISHED' AND published_at IS NOT NULL AND published_by IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uq_subscription_plan_versions_draft
    ON subscription_plan_versions (plan_id)
    WHERE status = 'DRAFT';
CREATE INDEX ix_subscription_plan_versions_plan_status
    ON subscription_plan_versions (plan_id, status, version DESC);

CREATE TABLE subscription_plan_entitlements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_version_id uuid NOT NULL
        REFERENCES subscription_plan_versions(id)
        ON UPDATE CASCADE ON DELETE CASCADE,
    feature_key varchar(120) NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    limit_value bigint,
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_subscription_plan_entitlements
        UNIQUE (plan_version_id, feature_key),
    CONSTRAINT chk_subscription_plan_entitlements_key
        CHECK (feature_key ~ '^[a-z0-9]+(?:[._-][a-z0-9]+)*$'),
    CONSTRAINT chk_subscription_plan_entitlements_limit
        CHECK (limit_value IS NULL OR limit_value >= 0),
    CONSTRAINT chk_subscription_plan_entitlements_configuration
        CHECK (jsonb_typeof(configuration) = 'object')
);

CREATE TABLE cpo_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    plan_version_id uuid NOT NULL
        REFERENCES subscription_plan_versions(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    status varchar(20) NOT NULL,
    starts_at timestamptz NOT NULL,
    trial_ends_at timestamptz,
    current_period_starts_at timestamptz NOT NULL,
    current_period_ends_at timestamptz NOT NULL,
    cancel_at_period_end boolean NOT NULL DEFAULT false,
    pending_plan_version_id uuid
        REFERENCES subscription_plan_versions(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    pending_change_at timestamptz,
    pending_change_by uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cancellation_scheduled_by uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cancelled_at timestamptz,
    ended_at timestamptz,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_cpo_subscriptions_status
        CHECK (status IN (
            'TRIAL', 'ACTIVE', 'PAUSED', 'PAST_DUE', 'CANCELLED', 'EXPIRED'
        )),
    CONSTRAINT chk_cpo_subscriptions_period
        CHECK (current_period_ends_at > current_period_starts_at),
    CONSTRAINT chk_cpo_subscriptions_trial
        CHECK (trial_ends_at IS NULL OR trial_ends_at > starts_at),
    CONSTRAINT chk_cpo_subscriptions_pending_change CHECK (
        (
            pending_plan_version_id IS NULL
            AND pending_change_at IS NULL
            AND pending_change_by IS NULL
        )
        OR
        (
            pending_plan_version_id IS NOT NULL
            AND pending_change_at IS NOT NULL
            AND pending_change_by IS NOT NULL
        )
    ),
    CONSTRAINT chk_cpo_subscriptions_cancellation_actor CHECK (
        (cancel_at_period_end = FALSE AND cancellation_scheduled_by IS NULL)
        OR
        (cancel_at_period_end = TRUE AND cancellation_scheduled_by IS NOT NULL)
    ),
    CONSTRAINT chk_cpo_subscriptions_end_state CHECK (
        (status IN ('CANCELLED', 'EXPIRED') AND ended_at IS NOT NULL)
        OR
        (status NOT IN ('CANCELLED', 'EXPIRED') AND ended_at IS NULL)
    )
);

CREATE UNIQUE INDEX uq_cpo_subscriptions_current
    ON cpo_subscriptions (cpo_id)
    WHERE status IN ('TRIAL', 'ACTIVE', 'PAUSED', 'PAST_DUE');
CREATE INDEX ix_cpo_subscriptions_cpo_created
    ON cpo_subscriptions (cpo_id, created_at DESC);
CREATE INDEX ix_cpo_subscriptions_period_end
    ON cpo_subscriptions (status, current_period_ends_at);

CREATE TABLE cpo_subscription_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id uuid NOT NULL
        REFERENCES cpo_subscriptions(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    previous_status varchar(20),
    next_status varchar(20) NOT NULL,
    previous_plan_version_id uuid
        REFERENCES subscription_plan_versions(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    next_plan_version_id uuid NOT NULL
        REFERENCES subscription_plan_versions(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    actor_user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    reason varchar(500) NOT NULL,
    idempotency_key varchar(120) NOT NULL,
    effective_at timestamptz NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_subscription_history_idempotency
        UNIQUE (actor_user_id, idempotency_key),
    CONSTRAINT chk_cpo_subscription_history_previous_status
        CHECK (
            previous_status IS NULL OR previous_status IN (
                'TRIAL', 'ACTIVE', 'PAUSED', 'PAST_DUE', 'CANCELLED', 'EXPIRED'
            )
        ),
    CONSTRAINT chk_cpo_subscription_history_next_status
        CHECK (next_status IN (
            'TRIAL', 'ACTIVE', 'PAUSED', 'PAST_DUE', 'CANCELLED', 'EXPIRED'
        )),
    CONSTRAINT chk_cpo_subscription_history_reason CHECK (btrim(reason) <> ''),
    CONSTRAINT chk_cpo_subscription_history_key
        CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_cpo_subscription_history_metadata
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX ix_cpo_subscription_history_cpo_created
    ON cpo_subscription_history (cpo_id, created_at DESC);

CREATE TABLE cpo_entitlement_overrides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    feature_key varchar(120) NOT NULL,
    enabled boolean NOT NULL,
    limit_value bigint,
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb,
    reason varchar(500) NOT NULL,
    expires_at timestamptz,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_entitlement_overrides UNIQUE (cpo_id, feature_key),
    CONSTRAINT chk_cpo_entitlement_overrides_key
        CHECK (feature_key ~ '^[a-z0-9]+(?:[._-][a-z0-9]+)*$'),
    CONSTRAINT chk_cpo_entitlement_overrides_limit
        CHECK (limit_value IS NULL OR limit_value >= 0),
    CONSTRAINT chk_cpo_entitlement_overrides_reason CHECK (btrim(reason) <> ''),
    CONSTRAINT chk_cpo_entitlement_overrides_configuration
        CHECK (jsonb_typeof(configuration) = 'object')
);

CREATE INDEX ix_cpo_entitlement_overrides_cpo_expiry
    ON cpo_entitlement_overrides (cpo_id, expires_at);

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
            'CUSTOMER_PASSWORD_RESET_OTP',
            'CPO_SUBSCRIPTION_CHANGED'
        )
    );
