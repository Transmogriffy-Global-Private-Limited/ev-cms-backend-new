CREATE TABLE support_tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cpo_id uuid NOT NULL REFERENCES cpos(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    subject varchar(200) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'OPEN',
    created_by_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_support_tickets_subject CHECK (char_length(btrim(subject)) BETWEEN 1 AND 200),
    CONSTRAINT chk_support_tickets_status CHECK (status IN ('OPEN', 'PENDING', 'RESOLVED', 'CLOSED'))
);
CREATE INDEX ix_support_tickets_cpo_updated ON support_tickets (cpo_id, updated_at DESC, id DESC);

CREATE TABLE support_ticket_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id uuid NOT NULL REFERENCES support_tickets(id) ON UPDATE CASCADE ON DELETE CASCADE,
    author_user_id uuid NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    author_scope varchar(20) NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_support_ticket_messages_scope CHECK (author_scope IN ('CPO', 'PLATFORM')),
    CONSTRAINT chk_support_ticket_messages_body CHECK (char_length(btrim(body)) BETWEEN 1 AND 10000)
);
CREATE INDEX ix_support_ticket_messages_ticket_created ON support_ticket_messages (ticket_id, created_at ASC, id ASC);
