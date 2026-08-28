-- Replace the former broad CPO permission names with the precise source
-- controlled capabilities used by the application. Existing overrides are
-- expanded rather than discarded so an ALLOW/DENY retains its intended scope.
WITH expanded AS (
    SELECT
        override_row.membership_id,
        override_row.effect,
        override_row.created_by,
        override_row.created_at,
        override_row.updated_at,
        replacement.permission
    FROM cpo_membership_permission_overrides AS override_row
    CROSS JOIN LATERAL (
        VALUES
            ('network.read', 'hubs.read'),
            ('network.read', 'chargers.read'),
            ('network.manage', 'hubs.manage'),
            ('network.manage', 'chargers.manage'),
            ('commercial.read', 'tariffs.read'),
            ('commercial.read', 'customers.read'),
            ('commercial.manage', 'tariffs.manage'),
            ('commercial.manage', 'settings.manage'),
            ('operations.read', 'chargers.operations'),
            ('operations.read', 'charging_sessions.read'),
            ('operations.manage', 'chargers.operations'),
            ('support.manage', 'support.create'),
            ('support.manage', 'support.reply')
    ) AS replacement(old_permission, permission)
    WHERE override_row.permission = replacement.old_permission
)
INSERT INTO cpo_membership_permission_overrides (
    id, membership_id, permission, effect, created_by, created_at, updated_at
)
SELECT gen_random_uuid(), membership_id, permission, effect, created_by, created_at, updated_at
FROM expanded
ON CONFLICT (membership_id, permission) DO NOTHING;

DELETE FROM cpo_membership_permission_overrides
WHERE permission IN (
    'network.read', 'network.manage', 'commercial.read', 'commercial.manage',
    'operations.read', 'operations.manage', 'support.manage'
);
