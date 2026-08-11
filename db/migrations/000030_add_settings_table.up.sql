CREATE TABLE settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cpo_id UUID NOT NULL,
    invoice_logo VARCHAR(255),
    invoice_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cpo
        FOREIGN KEY(cpo_id)
        REFERENCES cpo(id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX ON settings (cpo_id);
