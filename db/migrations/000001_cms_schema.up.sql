CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(320) NOT NULL,
    password_hash varchar(255) NOT NULL,
    full_name varchar(255) NOT NULL,
    phone varchar(32),
    is_active boolean NOT NULL DEFAULT true,
    is_verified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_users_email_not_blank CHECK (btrim(email) <> ''),
    CONSTRAINT chk_users_full_name_not_blank CHECK (btrim(full_name) <> '')
);

CREATE UNIQUE INDEX uq_users_email_normalized ON users (lower(btrim(email)));

CREATE TABLE user_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL UNIQUE
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_user_settings_object CHECK (jsonb_typeof(settings) = 'object')
);

CREATE TABLE platform_admins (
    user_id uuid PRIMARY KEY
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cpos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug varchar(80) NOT NULL,
    business_name varchar(255) NOT NULL,
    company_type varchar(20) NOT NULL,
    gstin varchar(15),
    address text NOT NULL DEFAULT '',
    city varchar(100) NOT NULL DEFAULT '',
    state varchar(100) NOT NULL DEFAULT '',
    pincode varchar(10) NOT NULL DEFAULT '',
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_cpos_slug CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    CONSTRAINT chk_cpos_business_name_not_blank CHECK (btrim(business_name) <> ''),
    CONSTRAINT chk_cpos_company_type CHECK (company_type IN ('INDIVIDUAL', 'COMPANY')),
    CONSTRAINT chk_cpos_gstin CHECK (gstin IS NULL OR gstin ~ '^[0-9A-Z]{15}$'),
    CONSTRAINT chk_cpos_status CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED'))
);

CREATE UNIQUE INDEX uq_cpos_slug_normalized ON cpos (lower(slug));
CREATE UNIQUE INDEX uq_cpos_gstin_normalized
    ON cpos (upper(gstin))
    WHERE gstin IS NOT NULL;

CREATE TABLE cpo_memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    role varchar(20) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_cpo_membership UNIQUE (cpo_id, user_id),
    CONSTRAINT chk_cpo_memberships_role
        CHECK (role IN ('OWNER', 'ADMIN', 'OPERATOR', 'VIEWER')),
    CONSTRAINT chk_cpo_memberships_status
        CHECK (status IN ('ACTIVE', 'SUSPENDED', 'REVOKED'))
);

CREATE INDEX ix_cpo_memberships_cpo_status
    ON cpo_memberships (cpo_id, status);
CREATE INDEX ix_cpo_memberships_user
    ON cpo_memberships (user_id);

CREATE TABLE user_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    name varchar(100) NOT NULL,
    description text NOT NULL DEFAULT '',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_groups_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT chk_user_groups_name_not_blank CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX uq_user_groups_cpo_name
    ON user_groups (cpo_id, lower(btrim(name)));

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    user_id uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    user_group_id uuid,
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_customers_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_cpo_customer UNIQUE (cpo_id, user_id),
    CONSTRAINT fk_customers_user_group
        FOREIGN KEY (cpo_id, user_group_id)
        REFERENCES user_groups(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_customers_status CHECK (status IN ('ACTIVE', 'BLOCKED'))
);

CREATE INDEX ix_customers_cpo_status ON customers (cpo_id, status);
CREATE INDEX ix_customers_user ON customers (user_id);
CREATE INDEX ix_customers_user_group ON customers (cpo_id, user_group_id);

CREATE TABLE hubs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    name varchar(255) NOT NULL,
    address text NOT NULL,
    latitude numeric(10,8) NOT NULL,
    longitude numeric(11,8) NOT NULL,
    open_24_hours boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_hubs_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT chk_hubs_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT chk_hubs_address_not_blank CHECK (btrim(address) <> ''),
    CONSTRAINT chk_hubs_latitude CHECK (latitude BETWEEN -90 AND 90),
    CONSTRAINT chk_hubs_longitude CHECK (longitude BETWEEN -180 AND 180)
);

CREATE INDEX ix_hubs_cpo ON hubs (cpo_id);
CREATE INDEX ix_hubs_location ON hubs (latitude, longitude);

CREATE TABLE chargers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    hub_id uuid NOT NULL,
    charger_id varchar(6) NOT NULL,
    ocpp_identity varchar(255) NOT NULL,
    vendor varchar(100) NOT NULL DEFAULT '',
    model varchar(100) NOT NULL DEFAULT '',
    serial_number varchar(100) NOT NULL DEFAULT '',
    max_power_kw numeric(8,2) NOT NULL DEFAULT 0,
    status varchar(30) NOT NULL DEFAULT 'OFFLINE',
    ocpp_version varchar(20) NOT NULL DEFAULT '1.6J',
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_chargers_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_chargers_cpo_hub_id_id UNIQUE (cpo_id, hub_id, id),
    CONSTRAINT fk_chargers_hub
        FOREIGN KEY (cpo_id, hub_id)
        REFERENCES hubs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_chargers_public_id CHECK (charger_id ~ '^[a-z0-9]{6}$'),
    CONSTRAINT chk_chargers_ocpp_identity CHECK (btrim(ocpp_identity) <> ''),
    CONSTRAINT chk_chargers_max_power CHECK (max_power_kw >= 0),
    CONSTRAINT chk_chargers_status CHECK (
        status IN (
            'AVAILABLE',
            'PREPARING',
            'CHARGING',
            'SUSPENDED_EV',
            'SUSPENDED_EVSE',
            'FINISHING',
            'RESERVED',
            'UNAVAILABLE',
            'FAULTED',
            'OFFLINE'
        )
    )
);

CREATE UNIQUE INDEX uq_chargers_public_id ON chargers (charger_id);
CREATE UNIQUE INDEX uq_chargers_ocpp_identity ON chargers (ocpp_identity);
CREATE INDEX ix_chargers_cpo_hub ON chargers (cpo_id, hub_id);
CREATE INDEX ix_chargers_cpo_status ON chargers (cpo_id, status);

CREATE TABLE connectors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    charger_id uuid NOT NULL,
    connector_number integer NOT NULL,
    connector_type varchar(50) NOT NULL,
    max_current numeric(8,2) NOT NULL DEFAULT 0,
    max_voltage numeric(8,2) NOT NULL DEFAULT 0,
    status varchar(30) NOT NULL DEFAULT 'AVAILABLE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_connectors_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_connectors_cpo_charger_id_id UNIQUE (cpo_id, charger_id, id),
    CONSTRAINT uq_connectors_charger_number
        UNIQUE (cpo_id, charger_id, connector_number),
    CONSTRAINT fk_connectors_charger
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_connectors_number CHECK (connector_number > 0),
    CONSTRAINT chk_connectors_type CHECK (btrim(connector_type) <> ''),
    CONSTRAINT chk_connectors_current CHECK (max_current >= 0),
    CONSTRAINT chk_connectors_voltage CHECK (max_voltage >= 0),
    CONSTRAINT chk_connectors_status CHECK (
        status IN (
            'AVAILABLE',
            'PREPARING',
            'CHARGING',
            'SUSPENDED_EV',
            'SUSPENDED_EVSE',
            'FINISHING',
            'RESERVED',
            'UNAVAILABLE',
            'FAULTED',
            'OFFLINE'
        )
    )
);

CREATE INDEX ix_connectors_cpo_charger ON connectors (cpo_id, charger_id);
CREATE INDEX ix_connectors_cpo_status ON connectors (cpo_id, status);

CREATE TABLE user_group_hubs (
    cpo_id uuid NOT NULL,
    user_group_id uuid NOT NULL,
    hub_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cpo_id, user_group_id, hub_id),
    CONSTRAINT fk_user_group_hubs_group
        FOREIGN KEY (cpo_id, user_group_id)
        REFERENCES user_groups(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_user_group_hubs_hub
        FOREIGN KEY (cpo_id, hub_id)
        REFERENCES hubs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE TABLE user_group_chargers (
    cpo_id uuid NOT NULL,
    user_group_id uuid NOT NULL,
    charger_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cpo_id, user_group_id, charger_id),
    CONSTRAINT fk_user_group_chargers_group
        FOREIGN KEY (cpo_id, user_group_id)
        REFERENCES user_groups(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_user_group_chargers_charger
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE TABLE customer_favorite_hubs (
    cpo_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    hub_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cpo_id, customer_id, hub_id),
    CONSTRAINT fk_customer_favorite_hubs_customer
        FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_favorite_hubs_hub
        FOREIGN KEY (cpo_id, hub_id)
        REFERENCES hubs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE TABLE customer_favorite_chargers (
    cpo_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    charger_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cpo_id, customer_id, charger_id),
    CONSTRAINT fk_customer_favorite_chargers_customer
        FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_customer_favorite_chargers_charger
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE TABLE gsts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    name varchar(100) NOT NULL,
    sgst_rate numeric(5,2) NOT NULL DEFAULT 9.00,
    cgst_rate numeric(5,2) NOT NULL DEFAULT 9.00,
    igst_rate numeric(5,2) NOT NULL DEFAULT 18.00,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_gsts_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT chk_gsts_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT chk_gsts_sgst_rate CHECK (sgst_rate BETWEEN 0 AND 100),
    CONSTRAINT chk_gsts_cgst_rate CHECK (cgst_rate BETWEEN 0 AND 100),
    CONSTRAINT chk_gsts_igst_rate CHECK (igst_rate BETWEEN 0 AND 100)
);

CREATE UNIQUE INDEX uq_gsts_cpo_name ON gsts (cpo_id, lower(btrim(name)));

CREATE TABLE tariffs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    hub_id uuid NOT NULL,
    charger_id uuid,
    gst_id uuid,
    user_group_id uuid,
    price_per_kwh numeric(12,4) NOT NULL,
    idle_fee_per_min numeric(12,4) NOT NULL DEFAULT 0,
    currency char(3) NOT NULL DEFAULT 'INR',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_tariffs_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT fk_tariffs_hub
        FOREIGN KEY (cpo_id, hub_id)
        REFERENCES hubs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_tariffs_charger
        FOREIGN KEY (cpo_id, hub_id, charger_id)
        REFERENCES chargers(cpo_id, hub_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_tariffs_gst
        FOREIGN KEY (cpo_id, gst_id)
        REFERENCES gsts(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_tariffs_user_group
        FOREIGN KEY (cpo_id, user_group_id)
        REFERENCES user_groups(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_tariffs_price CHECK (price_per_kwh >= 0),
    CONSTRAINT chk_tariffs_idle_fee CHECK (idle_fee_per_min >= 0),
    CONSTRAINT chk_tariffs_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX ix_tariffs_cpo_hub_active ON tariffs (cpo_id, hub_id, is_active);
CREATE INDEX ix_tariffs_cpo_charger ON tariffs (cpo_id, charger_id);
CREATE INDEX ix_tariffs_cpo_user_group ON tariffs (cpo_id, user_group_id);

CREATE TABLE wallets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    balance numeric(14,2) NOT NULL DEFAULT 0,
    currency char(3) NOT NULL DEFAULT 'INR',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_wallets_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_wallets_customer UNIQUE (cpo_id, customer_id),
    CONSTRAINT fk_wallets_customer
        FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_wallets_balance CHECK (balance >= 0),
    CONSTRAINT chk_wallets_currency CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE TABLE charging_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    transaction_id integer NOT NULL,
    customer_id uuid NOT NULL,
    charger_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    tariff_id uuid NOT NULL,
    start_time timestamptz NOT NULL,
    end_time timestamptz,
    meter_start_wh bigint NOT NULL,
    meter_stop_wh bigint,
    total_kwh numeric(14,3) NOT NULL DEFAULT 0,
    total_amount numeric(14,2) NOT NULL DEFAULT 0,
    currency char(3) NOT NULL DEFAULT 'INR',
    stop_reason varchar(50),
    tariff_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    tax_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(30) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_charging_sessions_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT uq_charging_sessions_transaction
        UNIQUE (cpo_id, charger_id, transaction_id),
    CONSTRAINT fk_charging_sessions_customer
        FOREIGN KEY (cpo_id, customer_id)
        REFERENCES customers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_charging_sessions_charger
        FOREIGN KEY (cpo_id, charger_id)
        REFERENCES chargers(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_charging_sessions_connector
        FOREIGN KEY (cpo_id, charger_id, connector_id)
        REFERENCES connectors(cpo_id, charger_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_charging_sessions_tariff
        FOREIGN KEY (cpo_id, tariff_id)
        REFERENCES tariffs(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_charging_sessions_transaction CHECK (transaction_id > 0),
    CONSTRAINT chk_charging_sessions_meter_start CHECK (meter_start_wh >= 0),
    CONSTRAINT chk_charging_sessions_meter_stop
        CHECK (meter_stop_wh IS NULL OR meter_stop_wh >= meter_start_wh),
    CONSTRAINT chk_charging_sessions_total_kwh CHECK (total_kwh >= 0),
    CONSTRAINT chk_charging_sessions_total_amount CHECK (total_amount >= 0),
    CONSTRAINT chk_charging_sessions_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_charging_sessions_times
        CHECK (end_time IS NULL OR end_time >= start_time),
    CONSTRAINT chk_charging_sessions_tariff_snapshot
        CHECK (jsonb_typeof(tariff_snapshot) = 'object'),
    CONSTRAINT chk_charging_sessions_tax_snapshot
        CHECK (jsonb_typeof(tax_snapshot) = 'object'),
    CONSTRAINT chk_charging_sessions_status CHECK (
        status IN (
            'START_PENDING',
            'ACTIVE',
            'STOP_PENDING',
            'COMPLETED',
            'FAILED'
        )
    )
);

CREATE INDEX ix_charging_sessions_cpo_customer
    ON charging_sessions (cpo_id, customer_id, start_time DESC);
CREATE INDEX ix_charging_sessions_cpo_charger_status
    ON charging_sessions (cpo_id, charger_id, status);
CREATE INDEX ix_charging_sessions_cpo_connector
    ON charging_sessions (cpo_id, connector_id);

CREATE TABLE wallet_transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    wallet_id uuid NOT NULL,
    session_id uuid,
    amount numeric(14,2) NOT NULL,
    transaction_type varchar(20) NOT NULL,
    description varchar(255) NOT NULL DEFAULT '',
    idempotency_key varchar(100),
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_wallet_transactions_cpo_id_id UNIQUE (cpo_id, id),
    CONSTRAINT fk_wallet_transactions_wallet
        FOREIGN KEY (cpo_id, wallet_id)
        REFERENCES wallets(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_wallet_transactions_session
        FOREIGN KEY (cpo_id, session_id)
        REFERENCES charging_sessions(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_wallet_transactions_amount CHECK (amount > 0),
    CONSTRAINT chk_wallet_transactions_type
        CHECK (transaction_type IN ('CREDIT', 'DEBIT')),
    CONSTRAINT chk_wallet_transactions_status
        CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED', 'REVERSED'))
);

CREATE UNIQUE INDEX uq_wallet_transactions_idempotency
    ON wallet_transactions (cpo_id, wallet_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX ix_wallet_transactions_wallet_created
    ON wallet_transactions (cpo_id, wallet_id, created_at DESC);
CREATE INDEX ix_wallet_transactions_session
    ON wallet_transactions (cpo_id, session_id);

CREATE TABLE payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL,
    session_id uuid NOT NULL,
    wallet_transaction_id uuid NOT NULL,
    amount numeric(14,2) NOT NULL,
    payment_method varchar(20) NOT NULL DEFAULT 'WALLET',
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_payments_session UNIQUE (cpo_id, session_id),
    CONSTRAINT uq_payments_wallet_transaction UNIQUE (cpo_id, wallet_transaction_id),
    CONSTRAINT fk_payments_session
        FOREIGN KEY (cpo_id, session_id)
        REFERENCES charging_sessions(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_payments_wallet_transaction
        FOREIGN KEY (cpo_id, wallet_transaction_id)
        REFERENCES wallet_transactions(cpo_id, id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT chk_payments_amount CHECK (amount >= 0),
    CONSTRAINT chk_payments_method CHECK (payment_method = 'WALLET'),
    CONSTRAINT chk_payments_status
        CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED', 'REVERSED', 'REFUNDED'))
);

CREATE INDEX ix_payments_cpo_status ON payments (cpo_id, status);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    user_id uuid
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL,
    action varchar(100) NOT NULL,
    entity varchar(100) NOT NULL,
    entity_id uuid,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_audit_logs_action_not_blank CHECK (btrim(action) <> ''),
    CONSTRAINT chk_audit_logs_entity_not_blank CHECK (btrim(entity) <> ''),
    CONSTRAINT chk_audit_logs_details_object CHECK (jsonb_typeof(details) = 'object')
);

CREATE INDEX ix_audit_logs_cpo_created
    ON audit_logs (cpo_id, created_at DESC);
CREATE INDEX ix_audit_logs_user_created
    ON audit_logs (user_id, created_at DESC);
CREATE INDEX ix_audit_logs_entity
    ON audit_logs (entity, entity_id);
