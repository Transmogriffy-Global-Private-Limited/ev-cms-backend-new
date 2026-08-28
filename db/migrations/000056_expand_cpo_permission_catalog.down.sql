-- The precise entries are intentionally not collapsed back into broad entries:
-- reverse mapping would create authority that was not explicitly present.
DELETE FROM cpo_membership_permission_overrides
WHERE permission IN (
    'hubs.read', 'hubs.manage', 'chargers.read', 'chargers.manage',
    'tariffs.read', 'tariffs.manage', 'charging_sessions.read',
    'support.create', 'support.reply', 'settings.manage'
);
