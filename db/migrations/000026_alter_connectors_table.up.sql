ALTER TABLE connectors ADD COLUMN connector_total_capacity NUMERIC(10, 2) NOT NULL DEFAULT 0;
ALTER TABLE connectors DROP COLUMN max_current;
ALTER TABLE connectors DROP COLUMN max_voltage;
