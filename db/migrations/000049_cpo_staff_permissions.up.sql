-- Source code owns the permission catalog and default role policy.  This
-- table stores only per-membership exceptions, with a single explicit effect.
CREATE TABLE cpo_membership_permission_overrides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    membership_id uuid NOT NULL REFERENCES cpo_memberships(id) ON UPDATE CASCADE ON DELETE CASCADE,
    permission varchar(100) NOT NULL,
    effect varchar(10) NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_membership_permission_override UNIQUE (membership_id, permission),
    CONSTRAINT chk_cpo_membership_permission_override_effect CHECK (effect IN ('ALLOW', 'DENY')),
    CONSTRAINT chk_cpo_membership_permission_override_permission CHECK (char_length(btrim(permission)) BETWEEN 1 AND 100)
);

CREATE INDEX ix_cpo_membership_permission_overrides_membership
    ON cpo_membership_permission_overrides (membership_id, permission);
