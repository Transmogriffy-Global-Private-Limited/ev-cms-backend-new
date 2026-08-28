DROP TABLE IF EXISTS support_ticket_events;
ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS chk_support_tickets_status;
ALTER TABLE support_tickets ADD CONSTRAINT chk_support_tickets_status CHECK (status IN ('OPEN','PENDING','RESOLVED','CLOSED'));
UPDATE support_tickets SET status = 'PENDING' WHERE status = 'IN_PROGRESS';
