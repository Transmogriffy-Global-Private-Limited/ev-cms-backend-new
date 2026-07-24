CREATE TABLE cpo_billing_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL UNIQUE
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    legal_name varchar(255) NOT NULL,
    billing_email varchar(320) NOT NULL,
    tax_id varchar(50),
    currency char(3) NOT NULL,
    billing_address jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_cpo_billing_accounts_legal_name
        CHECK (btrim(legal_name) <> ''),
    CONSTRAINT chk_cpo_billing_accounts_email
        CHECK (btrim(billing_email) <> ''),
    CONSTRAINT chk_cpo_billing_accounts_currency
        CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_cpo_billing_accounts_address
        CHECK (jsonb_typeof(billing_address) = 'object')
);

CREATE TABLE platform_invoices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_number varchar(80) NOT NULL UNIQUE,
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    billing_account_id uuid NOT NULL
        REFERENCES cpo_billing_accounts(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    subscription_id uuid
        REFERENCES cpo_subscriptions(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    currency char(3) NOT NULL,
    status varchar(30) NOT NULL DEFAULT 'DRAFT',
    subtotal_minor bigint NOT NULL DEFAULT 0,
    tax_minor bigint NOT NULL DEFAULT 0,
    total_minor bigint NOT NULL DEFAULT 0,
    paid_minor bigint NOT NULL DEFAULT 0,
    due_minor bigint NOT NULL DEFAULT 0,
    period_starts_at timestamptz,
    period_ends_at timestamptz,
    issued_at timestamptz,
    due_at timestamptz,
    voided_at timestamptz,
    void_reason varchar(500),
    external_reference varchar(255),
    idempotency_key varchar(120) NOT NULL,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_platform_invoices_idempotency
        UNIQUE (created_by, idempotency_key),
    CONSTRAINT chk_platform_invoices_number CHECK (btrim(invoice_number) <> ''),
    CONSTRAINT chk_platform_invoices_currency
        CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_platform_invoices_status CHECK (
        status IN (
            'DRAFT', 'ISSUED', 'PARTIALLY_PAID', 'PAID', 'OVERDUE', 'VOID'
        )
    ),
    CONSTRAINT chk_platform_invoices_amounts CHECK (
        subtotal_minor >= 0
        AND tax_minor >= 0
        AND total_minor >= 0
        AND paid_minor >= 0
        AND due_minor >= 0
        AND total_minor = subtotal_minor + tax_minor
        AND paid_minor + due_minor = total_minor
    ),
    CONSTRAINT chk_platform_invoices_period CHECK (
        period_starts_at IS NULL
        OR period_ends_at IS NULL
        OR period_ends_at > period_starts_at
    ),
    CONSTRAINT chk_platform_invoices_issue_state CHECK (
        (status = 'DRAFT' AND issued_at IS NULL AND due_at IS NULL)
        OR
        (status <> 'DRAFT' AND issued_at IS NOT NULL AND due_at IS NOT NULL)
    ),
    CONSTRAINT chk_platform_invoices_void_state CHECK (
        (status = 'VOID' AND voided_at IS NOT NULL AND btrim(void_reason) <> '')
        OR
        (status <> 'VOID' AND voided_at IS NULL AND void_reason IS NULL)
    )
);

CREATE UNIQUE INDEX uq_platform_invoices_external_reference
    ON platform_invoices (external_reference)
    WHERE external_reference IS NOT NULL;
CREATE INDEX ix_platform_invoices_cpo_created
    ON platform_invoices (cpo_id, created_at DESC);
CREATE INDEX ix_platform_invoices_status_due
    ON platform_invoices (status, due_at);

CREATE TABLE platform_invoice_lines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id uuid NOT NULL
        REFERENCES platform_invoices(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    line_number integer NOT NULL,
    description varchar(500) NOT NULL,
    quantity bigint NOT NULL,
    unit_amount_minor bigint NOT NULL,
    subtotal_minor bigint NOT NULL,
    tax_minor bigint NOT NULL DEFAULT 0,
    total_minor bigint NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_platform_invoice_lines_number
        UNIQUE (invoice_id, line_number),
    CONSTRAINT chk_platform_invoice_lines_description
        CHECK (btrim(description) <> ''),
    CONSTRAINT chk_platform_invoice_lines_amounts CHECK (
        quantity > 0
        AND unit_amount_minor >= 0
        AND subtotal_minor = quantity * unit_amount_minor
        AND tax_minor >= 0
        AND total_minor = subtotal_minor + tax_minor
    ),
    CONSTRAINT chk_platform_invoice_lines_metadata
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX ix_platform_invoice_lines_invoice
    ON platform_invoice_lines (invoice_id, line_number);

CREATE TABLE platform_payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_reference varchar(120) NOT NULL UNIQUE,
    cpo_id uuid NOT NULL
        REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    currency char(3) NOT NULL,
    amount_minor bigint NOT NULL,
    allocated_minor bigint NOT NULL DEFAULT 0,
    status varchar(20) NOT NULL DEFAULT 'RECORDED',
    voided_at timestamptz,
    void_reason varchar(500),
    method varchar(50) NOT NULL,
    external_reference varchar(255),
    occurred_at timestamptz NOT NULL,
    notes varchar(1000) NOT NULL DEFAULT '',
    idempotency_key varchar(120) NOT NULL,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_platform_payments_idempotency
        UNIQUE (created_by, idempotency_key),
    CONSTRAINT chk_platform_payments_reference
        CHECK (btrim(payment_reference) <> ''),
    CONSTRAINT chk_platform_payments_currency
        CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_platform_payments_amounts CHECK (
        amount_minor > 0
        AND allocated_minor >= 0
        AND allocated_minor <= amount_minor
    ),
    CONSTRAINT chk_platform_payments_status
        CHECK (status IN ('RECORDED', 'VOID')),
    CONSTRAINT chk_platform_payments_void_state CHECK (
        (status = 'VOID' AND voided_at IS NOT NULL AND btrim(void_reason) <> '')
        OR
        (status <> 'VOID' AND voided_at IS NULL AND void_reason IS NULL)
    ),
    CONSTRAINT chk_platform_payments_method CHECK (btrim(method) <> '')
);

CREATE UNIQUE INDEX uq_platform_payments_external_reference
    ON platform_payments (external_reference)
    WHERE external_reference IS NOT NULL;
CREATE INDEX ix_platform_payments_cpo_created
    ON platform_payments (cpo_id, created_at DESC);

CREATE TABLE platform_payment_allocations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id uuid NOT NULL
        REFERENCES platform_payments(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    invoice_id uuid NOT NULL
        REFERENCES platform_invoices(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    amount_minor bigint NOT NULL,
    created_by uuid NOT NULL
        REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_platform_payment_allocations
        UNIQUE (payment_id, invoice_id),
    CONSTRAINT chk_platform_payment_allocations_amount
        CHECK (amount_minor > 0)
);

CREATE INDEX ix_platform_payment_allocations_invoice
    ON platform_payment_allocations (invoice_id, created_at);

CREATE FUNCTION protect_issued_platform_invoice()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status <> 'DRAFT' AND (
        NEW.invoice_number IS DISTINCT FROM OLD.invoice_number
        OR NEW.cpo_id IS DISTINCT FROM OLD.cpo_id
        OR NEW.billing_account_id IS DISTINCT FROM OLD.billing_account_id
        OR NEW.subscription_id IS DISTINCT FROM OLD.subscription_id
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.subtotal_minor IS DISTINCT FROM OLD.subtotal_minor
        OR NEW.tax_minor IS DISTINCT FROM OLD.tax_minor
        OR NEW.total_minor IS DISTINCT FROM OLD.total_minor
        OR NEW.period_starts_at IS DISTINCT FROM OLD.period_starts_at
        OR NEW.period_ends_at IS DISTINCT FROM OLD.period_ends_at
        OR NEW.issued_at IS DISTINCT FROM OLD.issued_at
        OR NEW.due_at IS DISTINCT FROM OLD.due_at
        OR NEW.external_reference IS DISTINCT FROM OLD.external_reference
    ) THEN
        RAISE EXCEPTION 'issued platform invoice commercial terms are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_platform_invoices_protect_issued
BEFORE UPDATE ON platform_invoices
FOR EACH ROW
EXECUTE FUNCTION protect_issued_platform_invoice();

CREATE FUNCTION protect_platform_invoice_line()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    invoice_status varchar(30);
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT status
          INTO invoice_status
          FROM platform_invoices
         WHERE id = OLD.invoice_id;
    ELSE
        SELECT status
          INTO invoice_status
          FROM platform_invoices
         WHERE id = NEW.invoice_id;
    END IF;
    IF invoice_status <> 'DRAFT' THEN
        RAISE EXCEPTION 'issued platform invoice lines are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_platform_invoice_lines_protect_issued
BEFORE UPDATE OR DELETE ON platform_invoice_lines
FOR EACH ROW
EXECUTE FUNCTION protect_platform_invoice_line();

ALTER TABLE mail_outbox
    DROP CONSTRAINT chk_mail_outbox_template,
    ADD CONSTRAINT chk_mail_outbox_template CHECK (
        template IN (
            'LOGIN_OTP',
            'PASSWORD_RESET_OTP',
            'CPO_ADMIN_WELCOME',
            'CPO_MEMBERSHIP_ASSIGNED',
            'PASSWORD_CHANGE_REMINDER',
            'CUSTOMER_SIGNUP_OTP',
            'CUSTOMER_LOGIN_OTP',
            'CUSTOMER_PASSWORD_RESET_OTP',
            'CPO_SUBSCRIPTION_CHANGED',
            'CPO_PLATFORM_INVOICE_ISSUED'
        )
    );
