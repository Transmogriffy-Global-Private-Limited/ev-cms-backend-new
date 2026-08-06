ALTER TABLE connectors DROP COLUMN connector_total_capacity;
ALTER TABLE connectors ADD COLUMN max_current NUMERIC(8, 2) NOT NULL DEFAULT 0;
ALTER TABLE connectors ADD COLUMN max_voltage NUMERIC(8, 2) NOT NULL DEFAULT 0;
