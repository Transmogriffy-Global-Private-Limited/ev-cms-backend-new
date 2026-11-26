-- CoW is off for every existing CPO until a Platform administrator grants it.
-- No global feature switch or role claim can activate a missing entitlement.
CREATE TABLE cpo_module_entitlements (
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    module_key varchar(40) NOT NULL CHECK (module_key = 'cow'),
    enabled boolean NOT NULL DEFAULT false,
    version bigint NOT NULL CHECK (version > 0),
    reason varchar(500) NOT NULL CHECK (length(btrim(reason)) BETWEEN 3 AND 500),
    changed_by_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    changed_at timestamptz NOT NULL,
    PRIMARY KEY (cpo_id, module_key)
);

-- Append-only application history. Each state-changing transition records one
-- version; an unchanged PUT does not fabricate an event.
CREATE TABLE cpo_module_entitlement_events (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL,
    module_key varchar(40) NOT NULL CHECK (module_key = 'cow'),
    version bigint NOT NULL CHECK (version > 0),
    old_enabled boolean NOT NULL,
    new_enabled boolean NOT NULL,
    changed_by_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    reason varchar(500) NOT NULL CHECK (length(btrim(reason)) BETWEEN 3 AND 500),
    occurred_at timestamptz NOT NULL,
    CONSTRAINT uq_cpo_module_entitlement_event_version UNIQUE (cpo_id, module_key, version),
    CONSTRAINT fk_cpo_module_entitlement_event_grant
        FOREIGN KEY (cpo_id, module_key) REFERENCES cpo_module_entitlements(cpo_id, module_key)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_cpo_module_entitlement_event_transition CHECK (old_enabled <> new_enabled)
);
CREATE INDEX idx_cpo_module_entitlement_events_recent
    ON cpo_module_entitlement_events (cpo_id, module_key, occurred_at DESC);
