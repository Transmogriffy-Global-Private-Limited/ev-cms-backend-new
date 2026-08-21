ALTER TABLE hal_charger_mappings
    DROP CONSTRAINT IF EXISTS chk_hal_charger_mappings_last_sync_http_status,
    DROP COLUMN IF EXISTS last_sync_operation,
    DROP COLUMN IF EXISTS last_sync_correlation_id,
    DROP COLUMN IF EXISTS last_sync_provider_code,
    DROP COLUMN IF EXISTS last_sync_http_status,
    DROP COLUMN IF EXISTS last_sync_error_category;
