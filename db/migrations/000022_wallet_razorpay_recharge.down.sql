ALTER TABLE wallet_transactions
    DROP CONSTRAINT IF EXISTS fk_wallet_transactions_recharge_order;

DROP INDEX IF EXISTS uq_wallet_transactions_recharge_order;

ALTER TABLE wallet_transactions
    DROP COLUMN IF EXISTS recharge_order_id;

DROP TABLE IF EXISTS wallet_recharge_refunds;
DROP TABLE IF EXISTS wallet_recharge_payments;
DROP TABLE IF EXISTS wallet_recharge_orders;
