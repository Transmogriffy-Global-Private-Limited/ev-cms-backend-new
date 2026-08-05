CREATE TABLE wallet_recharge_orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    wallet_id uuid NOT NULL,
    provider varchar(30) NOT NULL DEFAULT 'RAZORPAY',
    idempotency_key varchar(120) NOT NULL,
    provider_order_id varchar(100),
    amount_minor bigint NOT NULL,
    currency char(3) NOT NULL DEFAULT 'INR',
    receipt varchar(40) NOT NULL,
    status varchar(30) NOT NULL DEFAULT 'PROVIDER_PENDING',
    provider_order_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    provider_created_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_wallet_recharge_orders_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_wallet_recharge_orders_idempotency
        UNIQUE (cpo_id, customer_id, idempotency_key),
    CONSTRAINT uq_wallet_recharge_orders_provider_order
        UNIQUE (cpo_id, provider_order_id),
    CONSTRAINT fk_wallet_recharge_orders_customer
        FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_wallet_recharge_orders_wallet
        FOREIGN KEY (cpo_id, wallet_id)
        REFERENCES wallets(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_wallet_recharge_orders_provider
        CHECK (provider = 'RAZORPAY'),
    CONSTRAINT chk_wallet_recharge_orders_amount
        CHECK (amount_minor > 0),
    CONSTRAINT chk_wallet_recharge_orders_currency
        CHECK (currency = 'INR'),
    CONSTRAINT chk_wallet_recharge_orders_status
        CHECK (status IN ('PROVIDER_PENDING', 'PAYMENT_PENDING', 'PAID', 'FAILED')),
    CONSTRAINT chk_wallet_recharge_orders_receipt
        CHECK (btrim(receipt) <> '')
);

CREATE INDEX ix_wallet_recharge_orders_cpo_customer_created
    ON wallet_recharge_orders (cpo_id, customer_id, created_at DESC);

CREATE TABLE wallet_recharge_payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    recharge_order_id uuid NOT NULL,
    provider_payment_id varchar(100) NOT NULL,
    provider_order_id varchar(100) NOT NULL,
    amount_minor bigint NOT NULL,
    currency char(3) NOT NULL,
    status varchar(30) NOT NULL,
    payment_method varchar(50),
    provider_fee_minor bigint,
    provider_tax_minor bigint,
    error_code varchar(100),
    error_description varchar(500),
    payment_signature varchar(128),
    signature_verified boolean NOT NULL DEFAULT false,
    provider_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    provider_created_at timestamptz,
    authorized_at timestamptz,
    captured_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_wallet_recharge_payments_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_wallet_recharge_payments_provider_id
        UNIQUE (cpo_id, provider_payment_id),
    CONSTRAINT fk_wallet_recharge_payments_order
        FOREIGN KEY (cpo_id, recharge_order_id)
        REFERENCES wallet_recharge_orders(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_wallet_recharge_payments_amount
        CHECK (amount_minor > 0),
    CONSTRAINT chk_wallet_recharge_payments_currency
        CHECK (currency = 'INR'),
    CONSTRAINT chk_wallet_recharge_payments_status
        CHECK (status IN ('AUTHORIZED', 'CAPTURED', 'FAILED'))
);

CREATE INDEX ix_wallet_recharge_payments_order_created
    ON wallet_recharge_payments (cpo_id, recharge_order_id, created_at DESC);

CREATE TABLE wallet_recharge_refunds (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    recharge_order_id uuid NOT NULL,
    recharge_payment_id uuid,
    provider_refund_id varchar(100),
    provider_payment_id varchar(100) NOT NULL,
    amount_minor bigint NOT NULL,
    currency char(3) NOT NULL,
    status varchar(30) NOT NULL,
    receipt varchar(40),
    speed_processed varchar(30),
    provider_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    provider_created_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_wallet_recharge_refunds_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_wallet_recharge_refunds_provider_id
        UNIQUE (cpo_id, provider_refund_id),
    CONSTRAINT fk_wallet_recharge_refunds_order
        FOREIGN KEY (cpo_id, recharge_order_id)
        REFERENCES wallet_recharge_orders(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_wallet_recharge_refunds_payment
        FOREIGN KEY (cpo_id, recharge_payment_id)
        REFERENCES wallet_recharge_payments(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_wallet_recharge_refunds_amount
        CHECK (amount_minor > 0),
    CONSTRAINT chk_wallet_recharge_refunds_currency
        CHECK (currency = 'INR'),
    CONSTRAINT chk_wallet_recharge_refunds_status
        CHECK (status IN ('PENDING', 'PROCESSED', 'FAILED'))
);

ALTER TABLE wallet_transactions
    ADD COLUMN recharge_order_id uuid;

ALTER TABLE wallet_transactions
    ADD CONSTRAINT fk_wallet_transactions_recharge_order
        FOREIGN KEY (cpo_id, recharge_order_id)
        REFERENCES wallet_recharge_orders(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT;

CREATE UNIQUE INDEX uq_wallet_transactions_recharge_order
    ON wallet_transactions (cpo_id, wallet_id, recharge_order_id)
    WHERE recharge_order_id IS NOT NULL;
