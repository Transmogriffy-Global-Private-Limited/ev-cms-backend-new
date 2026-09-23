ALTER TABLE support_ticket_messages
    DROP CONSTRAINT fk_support_ticket_messages_ticket_customer,
    ADD CONSTRAINT fk_support_ticket_messages_ticket_customer
        FOREIGN KEY (ticket_id, author_customer_id)
        REFERENCES support_tickets (id, customer_id)
        ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE support_ticket_events
    DROP CONSTRAINT fk_support_ticket_events_ticket_customer,
    ADD CONSTRAINT fk_support_ticket_events_ticket_customer
        FOREIGN KEY (ticket_id, actor_customer_id)
        REFERENCES support_tickets (id, customer_id)
        ON UPDATE CASCADE ON DELETE CASCADE;
