-- Existing SETTLED sessions deliberately remain NULL: their original
-- settlement time was not persisted, so migration time must not be invented.
-- New authoritative settlement transitions populate this fact in application
-- code and invoice email rollout uses it rather than charging end_time.
ALTER TABLE charging_sessions ADD COLUMN settled_at timestamptz;

CREATE TABLE charging_session_invoices (
    id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    customer_id uuid NOT NULL REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES charging_sessions(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    invoice_number varchar(20) NOT NULL,
    financial_year varchar(7) NOT NULL,
    serial bigint NOT NULL,
    issued_at timestamptz NOT NULL,
    snapshot_version integer NOT NULL DEFAULT 1,
    renderer_version varchar(40) NOT NULL,
    snapshot jsonb NOT NULL,
    generation_status varchar(20) NOT NULL DEFAULT 'PENDING',
    generation_attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    generation_locked_at timestamptz,
    last_generation_error varchar(500),
    storage_path varchar(500),
    mime_type varchar(100),
    file_size bigint,
    sha256 char(64),
    ready_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_charging_session_invoice_session UNIQUE (session_id),
    CONSTRAINT uq_charging_session_invoice_number UNIQUE (cpo_id, invoice_number),
    CONSTRAINT uq_charging_session_invoice_serial UNIQUE (cpo_id, financial_year, serial),
    CONSTRAINT chk_charging_session_invoice_financial_year CHECK (financial_year ~ '^[0-9]{2}-[0-9]{2}$'),
    CONSTRAINT chk_charging_session_invoice_serial CHECK (serial BETWEEN 1 AND 999999),
	CONSTRAINT chk_charging_session_invoice_number CHECK (invoice_number ~ '^INV/[0-9]{2}-[0-9]{2}/[0-9]{6}$'),
    CONSTRAINT chk_charging_session_invoice_state CHECK (generation_status IN ('PENDING', 'GENERATING', 'READY', 'FAILED', 'CORRUPT')),
    CONSTRAINT chk_charging_session_invoice_ready_metadata CHECK (
        (generation_status = 'READY' AND storage_path IS NOT NULL AND mime_type = 'application/pdf' AND file_size > 0 AND sha256 ~ '^[0-9a-f]{64}$' AND ready_at IS NOT NULL)
        OR generation_status <> 'READY'
    )
);

CREATE INDEX ix_charging_session_invoices_discovery
    ON charging_session_invoices (generation_status, available_at, created_at);
CREATE INDEX ix_charging_session_invoices_customer
    ON charging_session_invoices (cpo_id, customer_id, issued_at DESC);

CREATE TABLE invoice_assets (
    sha256 char(64) PRIMARY KEY,
    mime_type varchar(100) NOT NULL,
    content bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_invoice_asset_mime CHECK (mime_type IN ('image/png', 'image/jpeg')),
    CONSTRAINT chk_invoice_asset_hash CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE TABLE invoice_asset_references (
    invoice_id uuid PRIMARY KEY REFERENCES charging_session_invoices(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    sha256 char(64) NOT NULL REFERENCES invoice_assets(sha256) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invoice_number_sequences (
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    financial_year varchar(7) NOT NULL,
    next_serial bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cpo_id, financial_year),
    CONSTRAINT chk_invoice_number_sequence_year CHECK (financial_year ~ '^[0-9]{2}-[0-9]{2}$'),
    CONSTRAINT chk_invoice_number_sequence_next CHECK (next_serial BETWEEN 1 AND 1000000)
);

CREATE TABLE invoice_rollout (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    automatic_email_from timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO invoice_rollout (id, automatic_email_from) VALUES (1, now());

CREATE TABLE invoice_deliveries (
    id uuid PRIMARY KEY,
    invoice_id uuid NOT NULL REFERENCES charging_session_invoices(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    recipient varchar(320) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'PENDING',
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 8,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    last_error varchar(500),
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_invoice_delivery_invoice UNIQUE (invoice_id),
    CONSTRAINT chk_invoice_delivery_state CHECK (status IN ('PENDING', 'SENDING', 'SENT', 'FAILED', 'AMBIGUOUS')),
    CONSTRAINT chk_invoice_delivery_attempts CHECK (attempts >= 0 AND max_attempts > 0),
    CONSTRAINT chk_invoice_delivery_sent_at CHECK ((status = 'SENT' AND sent_at IS NOT NULL) OR status <> 'SENT')
);
CREATE INDEX ix_invoice_deliveries_claim ON invoice_deliveries (status, available_at, created_at);
