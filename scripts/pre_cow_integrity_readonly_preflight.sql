-- READ ONLY. Intended for an operator-selected, verified target at >= 000077.
-- Returns counts, not IDs, business names, customer details, or secrets.
-- ZERO invalid rows in each checks row is required for a safe upgrade.
-- The migration runner is still authoritative about the actual ledger.
BEGIN READ ONLY;
SELECT current_database() AS selected_database, current_user AS selected_role,
       current_setting('server_version_num') AS pg_version_num;
SELECT version, applied_at FROM schema_migrations
 WHERE version LIKE '00007%.up.sql' ORDER BY version;

WITH checks AS (
    SELECT '078_platform_duplicate_notification' AS check_name, count(*)::bigint AS invalid_rows
    FROM (SELECT announcement_id, recipient_user_id FROM platform_notifications
          WHERE cpo_id IS NULL GROUP BY announcement_id, recipient_user_id HAVING count(*)>1) d
    UNION ALL SELECT '078_notification_audience_mismatch', count(*)
    FROM platform_notifications n JOIN platform_announcements a ON a.id=n.announcement_id
    WHERE (a.audience='PLATFORM' AND n.cpo_id IS NOT NULL) OR (a.audience='CPO' AND n.cpo_id IS NULL)
    UNION ALL SELECT '078_notification_untargeted_cpo', count(*)
    FROM platform_notifications n JOIN platform_announcements a ON a.id=n.announcement_id
    WHERE a.audience='CPO' AND NOT EXISTS
          (SELECT 1 FROM platform_announcement_cpos t WHERE t.announcement_id=n.announcement_id AND t.cpo_id=n.cpo_id)
    UNION ALL SELECT '079_intent_customer_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN customers c ON c.cpo_id=i.cpo_id AND c.id=i.customer_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_intent_charger_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN chargers c ON c.cpo_id=i.cpo_id AND c.id=i.charger_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_intent_connector_charger_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN connectors c ON c.cpo_id=i.cpo_id AND c.charger_id=i.charger_id AND c.id=i.connector_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_intent_wallet_customer_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN wallets w ON w.cpo_id=i.cpo_id AND w.customer_id=i.customer_id AND w.id=i.wallet_id
    WHERE w.id IS NULL
    UNION ALL SELECT '079_intent_tariff_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN tariffs t ON t.cpo_id=i.cpo_id AND t.id=i.tariff_id
    WHERE t.id IS NULL
    UNION ALL SELECT '079_intent_materialized_session_tenant', count(*)
    FROM charging_start_intents i LEFT JOIN charging_sessions s ON s.cpo_id=i.cpo_id AND s.id=i.materialized_session_id
    WHERE i.materialized_session_id IS NOT NULL AND s.id IS NULL
    UNION ALL SELECT '079_hold_intent_wallet_tenant', count(*)
    FROM wallet_holds h LEFT JOIN charging_start_intents i ON i.cpo_id=h.cpo_id AND i.id=h.start_intent_id AND i.wallet_id=h.wallet_id
    WHERE i.id IS NULL
    UNION ALL SELECT '079_command_intent_tenant', count(*)
    FROM hal_command_records h LEFT JOIN charging_start_intents i ON i.cpo_id=h.cpo_id AND i.id=h.start_intent_id
    WHERE h.start_intent_id IS NOT NULL AND i.id IS NULL
    UNION ALL SELECT '079_command_session_tenant', count(*)
    FROM hal_command_records h LEFT JOIN charging_sessions s ON s.cpo_id=h.cpo_id AND s.id=h.charging_session_id
    WHERE h.charging_session_id IS NOT NULL AND s.id IS NULL
    UNION ALL SELECT '079_mapping_charger_tenant', count(*)
    FROM hal_charger_mappings h LEFT JOIN chargers c ON c.cpo_id=h.cpo_id AND c.id=h.cms_charger_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_runtime_charger_tenant', count(*)
    FROM hal_charger_runtime h LEFT JOIN chargers c ON c.cpo_id=h.cpo_id AND c.id=h.cms_charger_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_runtime_connector_tenant', count(*)
    FROM hal_connector_runtime h LEFT JOIN connectors c ON c.cpo_id=h.cpo_id AND c.charger_id=h.cms_charger_id AND c.id=h.cms_connector_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_session_intent_tenant', count(*)
    FROM charging_sessions s LEFT JOIN charging_start_intents i ON i.cpo_id=s.cpo_id AND i.id=s.start_intent_id
    WHERE s.start_intent_id IS NOT NULL AND i.id IS NULL
    UNION ALL SELECT '079_operation_charger_tenant', count(*)
    FROM charger_operations o LEFT JOIN chargers c ON c.cpo_id=o.cpo_id AND c.id=o.charger_id
    WHERE c.id IS NULL
    UNION ALL SELECT '079_operation_connector_tenant', count(*)
    FROM charger_operations o LEFT JOIN connectors c ON c.cpo_id=o.cpo_id AND c.charger_id=o.charger_id AND c.id=o.connector_id
    WHERE o.connector_id IS NOT NULL AND c.id IS NULL
    UNION ALL SELECT '079_invoice_session_customer_tenant', count(*)
    FROM charging_session_invoices i LEFT JOIN charging_sessions s ON s.cpo_id=i.cpo_id AND s.customer_id=i.customer_id AND s.id=i.session_id
    WHERE s.id IS NULL
)
SELECT check_name, invalid_rows, CASE WHEN invalid_rows=0 THEN 'PASS' ELSE 'BLOCK' END AS result
FROM checks ORDER BY check_name;

-- 000056's conflicting source permissions may already have been discarded.
-- This count is a REVIEW FLAG ONLY, not proof of a bad historical grant.
SELECT effect, count(*) AS rows_needing_historical_permission_provenance_review
FROM cpo_membership_permission_overrides
WHERE permission='chargers.operations'
GROUP BY effect ORDER BY effect;
COMMIT;
