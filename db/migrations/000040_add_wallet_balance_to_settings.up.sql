ALTER TABLE settings
ADD COLUMN wallet_min_balance INTEGER NOT NULL DEFAULT 0,
ADD COLUMN wallet_buffer_min_balance INTEGER NOT NULL DEFAULT 0;
