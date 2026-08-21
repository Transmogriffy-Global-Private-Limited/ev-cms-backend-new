-- Mapping retries are operational state, not charging-session truth. Keep only
-- bounded, safe diagnostics so an operator can distinguish retryable transport
-- failures from provider rejection without retaining provider payloads.
ALTER TABLE hal_charger_mappings
    ADD COLUMN last_sync_error_category varchar(32) NOT NULL DEFAULT '',
    ADD COLUMN last_sync_http_status integer,
    ADD COLUMN last_sync_provider_code varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN last_sync_correlation_id uuid,
    ADD COLUMN last_sync_operation varchar(64) NOT NULL DEFAULT '';

ALTER TABLE hal_charger_mappings
    ADD CONSTRAINT chk_hal_charger_mappings_last_sync_http_status
        CHECK (last_sync_http_status IS NULL OR (last_sync_http_status >= 100 AND last_sync_http_status <= 599));
