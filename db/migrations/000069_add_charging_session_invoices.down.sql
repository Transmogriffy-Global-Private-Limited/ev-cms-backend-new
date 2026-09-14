DROP TABLE IF EXISTS invoice_deliveries;
DROP TABLE IF EXISTS invoice_rollout;
DROP TABLE IF EXISTS invoice_number_sequences;
DROP TABLE IF EXISTS invoice_asset_references;
DROP TABLE IF EXISTS invoice_assets;
DROP TABLE IF EXISTS charging_session_invoices;
ALTER TABLE charging_sessions DROP COLUMN IF EXISTS settled_at;
