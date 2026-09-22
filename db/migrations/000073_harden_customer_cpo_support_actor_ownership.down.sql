ALTER TABLE support_ticket_events
    DROP CONSTRAINT fk_support_ticket_events_ticket_customer,
    ADD CONSTRAINT fk_support_ticket_events_customer
        FOREIGN KEY (actor_customer_id) REFERENCES customers(id)
        ON UPDATE CASCADE ON DELETE RESTRICT;

ALTER TABLE support_ticket_messages
    DROP CONSTRAINT fk_support_ticket_messages_ticket_customer,
    ADD CONSTRAINT fk_support_ticket_messages_customer
        FOREIGN KEY (author_customer_id) REFERENCES customers(id)
        ON UPDATE CASCADE ON DELETE RESTRICT;

ALTER TABLE support_tickets
    DROP CONSTRAINT uq_support_tickets_id_customer;
