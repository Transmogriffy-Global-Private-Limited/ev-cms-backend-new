DO $$ BEGIN
    IF EXISTS (
        SELECT 1
        FROM support_ticket_messages AS message
        JOIN support_tickets AS ticket ON ticket.id = message.ticket_id
        WHERE message.author_scope = 'CUSTOMER'
          AND message.author_customer_id IS DISTINCT FROM ticket.customer_id
    ) THEN
        RAISE EXCEPTION 'cannot harden customer support message actors while actor ownership does not match its ticket';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM support_ticket_events AS event
        JOIN support_tickets AS ticket ON ticket.id = event.ticket_id
        WHERE event.actor_scope = 'CUSTOMER'
          AND event.actor_customer_id IS DISTINCT FROM ticket.customer_id
    ) THEN
        RAISE EXCEPTION 'cannot harden customer support event actors while actor ownership does not match its ticket';
    END IF;
END $$;

ALTER TABLE support_tickets
    ADD CONSTRAINT uq_support_tickets_id_customer UNIQUE (id, customer_id);

ALTER TABLE support_ticket_messages
    DROP CONSTRAINT fk_support_ticket_messages_customer,
    ADD CONSTRAINT fk_support_ticket_messages_ticket_customer
        FOREIGN KEY (ticket_id, author_customer_id)
        REFERENCES support_tickets (id, customer_id)
        ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE support_ticket_events
    DROP CONSTRAINT fk_support_ticket_events_customer,
    ADD CONSTRAINT fk_support_ticket_events_ticket_customer
        FOREIGN KEY (ticket_id, actor_customer_id)
        REFERENCES support_tickets (id, customer_id)
        ON UPDATE CASCADE ON DELETE CASCADE;
