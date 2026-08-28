ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS chk_support_tickets_status;
UPDATE support_tickets SET status = 'IN_PROGRESS' WHERE status = 'PENDING';
ALTER TABLE support_tickets ADD CONSTRAINT chk_support_tickets_status CHECK (status IN ('OPEN','IN_PROGRESS','RESOLVED','CLOSED'));
CREATE TABLE support_ticket_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), ticket_id uuid NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
 event_type varchar(30) NOT NULL, actor_scope varchar(20) NOT NULL, actor_user_id uuid REFERENCES users(id),
 previous_status varchar(20), next_status varchar(20), reason varchar(500), idempotency_key varchar(120), created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT uq_support_ticket_event_idempotency UNIQUE(ticket_id,idempotency_key)
);
CREATE INDEX ix_support_ticket_events_ticket_created ON support_ticket_events(ticket_id,created_at,id);
