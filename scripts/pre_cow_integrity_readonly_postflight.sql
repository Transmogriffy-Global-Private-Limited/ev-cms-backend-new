-- READ ONLY. Verify applied ledger AND validation of every new FK.
-- Run only against an explicitly verified database already migrated to 000079.
BEGIN READ ONLY;
SELECT version, applied_at FROM schema_migrations
 WHERE version IN (
   '000078_pre_cow_notice_and_gst_integrity.up.sql',
   '000079_pre_cow_charging_tenant_integrity.up.sql'
 ) ORDER BY version;
SELECT conname AS constraint_name,
       conrelid::regclass::text AS owning_table,
       contype AS kind,
       convalidated AS validated,
       condeferrable AS deferrable
FROM pg_constraint
WHERE conname IN (
  'fk_hubs_gst',
  'fk_platform_notifications_announcement_target',
  'fk_start_intents_customer_tenant',
  'fk_start_intents_charger_tenant',
  'fk_start_intents_connector_tenant_charger',
  'fk_start_intents_wallet_customer_tenant',
  'fk_start_intents_tariff_tenant',
  'fk_start_intents_materialized_session_tenant',
  'fk_wallet_holds_intent_wallet_tenant',
  'fk_hal_commands_start_intent_tenant',
  'fk_hal_commands_session_tenant',
  'fk_hal_mappings_charger_tenant',
  'fk_hal_runtime_charger_tenant',
  'fk_hal_runtime_connector_tenant_charger',
  'fk_charging_sessions_start_intent_tenant',
  'fk_charger_operations_charger_tenant',
  'fk_charger_operations_connector_tenant_charger',
  'fk_session_invoice_exact_session_customer_tenant'
)
ORDER BY owning_table, constraint_name;
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname=current_schema()
AND indexname IN (
  'uq_platform_notifications_platform_recipient',
  'uq_wallets_cpo_customer_wallet_identity',
  'uq_start_intents_cpo_identity',
  'uq_start_intents_cpo_intent_wallet',
  'uq_charging_sessions_cpo_customer_session'
) ORDER BY indexname;
SELECT tgname, tgrelid::regclass::text AS owning_table, tgenabled
FROM pg_trigger
WHERE NOT tgisinternal AND tgname IN (
  'platform_notification_scope_guard',
  'platform_announcement_audience_guard'
) ORDER BY tgname;
COMMIT;
