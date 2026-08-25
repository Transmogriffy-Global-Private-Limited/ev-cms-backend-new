CREATE TABLE cpo_subscription_lifecycle_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id uuid NOT NULL REFERENCES cpo_subscriptions(id) ON UPDATE CASCADE ON DELETE CASCADE,
    kind varchar(30) NOT NULL,
    effective_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_subscription_lifecycle_event UNIQUE (subscription_id, kind),
    CONSTRAINT chk_cpo_subscription_lifecycle_event_kind CHECK (kind IN ('EXPIRY_WARNING_7D', 'EXPIRY_WARNING_3D', 'EXPIRY_WARNING_1D', 'EXPIRED'))
);

CREATE INDEX ix_cpo_subscription_lifecycle_events_subscription
    ON cpo_subscription_lifecycle_events (subscription_id, created_at DESC);
