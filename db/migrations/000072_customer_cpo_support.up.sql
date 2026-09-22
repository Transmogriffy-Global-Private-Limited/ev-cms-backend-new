ALTER TABLE support_tickets
    ADD COLUMN channel varchar(20) NOT NULL DEFAULT 'CPO_PLATFORM',
    ADD COLUMN customer_id uuid,
    ALTER COLUMN created_by_user_id DROP NOT NULL,
    ADD CONSTRAINT chk_support_tickets_channel CHECK (channel IN ('CPO_PLATFORM', 'CUSTOMER_CPO')),
    ADD CONSTRAINT fk_support_tickets_customer_cpo FOREIGN KEY (cpo_id, customer_id) REFERENCES customers(cpo_id, id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT chk_support_tickets_channel_actor CHECK ((channel = 'CPO_PLATFORM' AND customer_id IS NULL AND created_by_user_id IS NOT NULL) OR (channel = 'CUSTOMER_CPO' AND customer_id IS NOT NULL AND created_by_user_id IS NULL));
ALTER TABLE support_ticket_messages
    DROP CONSTRAINT IF EXISTS chk_support_ticket_messages_scope,
    ADD COLUMN author_customer_id uuid,
    ALTER COLUMN author_user_id DROP NOT NULL,
    ADD CONSTRAINT fk_support_ticket_messages_customer FOREIGN KEY (author_customer_id) REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT chk_support_ticket_messages_scope CHECK (author_scope IN ('CPO', 'PLATFORM', 'CUSTOMER')),
    ADD CONSTRAINT chk_support_ticket_messages_actor CHECK ((author_scope IN ('CPO', 'PLATFORM') AND author_user_id IS NOT NULL AND author_customer_id IS NULL) OR (author_scope = 'CUSTOMER' AND author_customer_id IS NOT NULL AND author_user_id IS NULL));
ALTER TABLE support_ticket_events
    ADD COLUMN actor_customer_id uuid,
    ADD CONSTRAINT fk_support_ticket_events_customer FOREIGN KEY (actor_customer_id) REFERENCES customers(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    ADD CONSTRAINT chk_support_ticket_events_actor CHECK ((actor_scope IN ('CPO', 'PLATFORM') AND actor_user_id IS NOT NULL AND actor_customer_id IS NULL) OR (actor_scope = 'CUSTOMER' AND actor_customer_id IS NOT NULL AND actor_user_id IS NULL));
CREATE INDEX ix_support_tickets_customer_updated ON support_tickets (cpo_id, customer_id, updated_at DESC, id DESC) WHERE channel = 'CUSTOMER_CPO';
CREATE INDEX ix_support_tickets_customer_cpo_updated ON support_tickets (cpo_id, updated_at DESC, id DESC) WHERE channel = 'CUSTOMER_CPO';
ALTER TABLE mail_outbox DROP CONSTRAINT IF EXISTS chk_mail_outbox_template;
ALTER TABLE mail_outbox ADD CONSTRAINT chk_mail_outbox_template CHECK (template IN ('LOGIN_OTP','PASSWORD_RESET_OTP','CUSTOMER_LOGIN_OTP','CUSTOMER_SIGNUP_OTP','CUSTOMER_PASSWORD_RESET_OTP','CPO_ADMIN_WELCOME','CPO_MEMBERSHIP_ASSIGNED','PASSWORD_CHANGE_REMINDER','PLATFORM_ADMIN_INVITE','PLATFORM_ADMIN_GRANTED','CPO_STAFF_NEW_IDENTITY','CPO_STAFF_EXISTING_IDENTITY','CPO_ONBOARDING_RESENT','CPO_STAFF_ROLE_CHANGED','CPO_STAFF_SUSPENDED','CPO_STAFF_REACTIVATED','CPO_STAFF_REVOKED','CPO_SUBSCRIPTION_EXPIRY_WARNING','CPO_SUBSCRIPTION_EXPIRED','CPO_SUPPORT_TICKET_CREATED','CPO_SUPPORT_TICKET_PLATFORM_REPLY','CPO_SUPPORT_TICKET_RESOLVED','CPO_SUPPORT_TICKET_CLOSED','CPO_SUPPORT_TICKET_REOPENED','CUSTOMER_CPO_SUPPORT_TICKET_CREATED','CUSTOMER_CPO_SUPPORT_TICKET_REPLY','CUSTOMER_CPO_SUPPORT_TICKET_STATUS_CHANGED')) NOT VALID;
