CREATE TABLE operational_events (
    id bigserial PRIMARY KEY,
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    customer_id uuid REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    event_type varchar(150) NOT NULL,
    resource_type varchar(100) NOT NULL,
    resource_id varchar(255) NOT NULL,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX ix_operational_events_cpo_cursor ON operational_events (cpo_id, id);
CREATE INDEX ix_operational_events_customer_cursor ON operational_events (customer_id, id) WHERE customer_id IS NOT NULL;
CREATE INDEX ix_operational_events_expires_at ON operational_events (expires_at);
