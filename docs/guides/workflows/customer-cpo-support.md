# Customer-to-CPO support workflow

## Scope and authority

This is the `CUSTOMER_CPO` support channel: an authenticated App Customer and
that customer's own CPO. It is not the existing `CPO_PLATFORM` channel between
CPO staff and Platform SuperAdmin. PostgreSQL support rows are authoritative;
mail is an encrypted, retrying outbox delivery hint and never ticket truth.

The customer bearer and matching `X-CPO-App-ID` derive both `cpo_id` and
`customer_id`. CPO staff derive the CPO from their bearer membership and the
matching App ID. Platform routes and `/api/v1/cpo/support` are constrained to
CPO_PLATFORM and cannot observe CUSTOMER_CPO rows.

## API and lifecycle

User App routes are `GET`/`POST /api/v1/app/support/tickets`, `GET
/api/v1/app/support/tickets/{ticket_id}`, and `POST
/api/v1/app/support/tickets/{ticket_id}/replies`. CPO routes are `GET
/api/v1/cpo/customer-support/tickets`, `GET
/api/v1/cpo/customer-support/tickets/{ticket_id}`, `POST
/api/v1/cpo/customer-support/tickets/{ticket_id}/replies`, and `PATCH
/api/v1/cpo/customer-support/tickets/{ticket_id}/status`.

Creation accepts only `{ "subject", "body" }`; both values are trimmed,
nonblank, and limited to 200 and 10,000 characters. Replies require the same
body validation plus a 1-120 character `idempotency_key`. Reuse that key after
an ambiguous response: the locked ticket/event uniqueness boundary returns the
already-current ticket without another message, event, or outbox intent.

Statuses are `OPEN`, `IN_PROGRESS`, `RESOLVED`, and `CLOSED`. CPO status
transitions use the existing graph. A CPO reply never changes status. A customer
reply to RESOLVED or CLOSED atomically changes it to OPEN, clears `closed_at`,
and records MESSAGE_ADDED plus STATUS_CHANGED evidence. Replies to OPEN or
IN_PROGRESS do not change status. Repeating a status is side-effect free.

## Visibility, filters, and privacy

Customer list/detail/reply operations are scoped by `(cpo_id, customer_id)` and
return privacy-safe `support_ticket_not_found` for another customer's or
another CPO's ticket. The User App sees only `CUSTOMER` or `CPO` author scope,
message body, status, and public lifecycle fields; it never receives an
individual CPO staff ID/email, permissions, or internal status reason.

CPO queue/detail requires `customer_support.read`; replies additionally require
`customer_support.reply`; status changes additionally require
`customer_support.manage`. The normal permission evaluator applies current
membership plus overrides and DENY wins. CPO views may expose only the owning
customer `{id, full_name, email}` and CPO staff audit IDs where needed. They do
not expose customer password, session, credential, phone, or wallet data.

Lists are SQL-filtered before `LIMIT`, ordered `updated_at DESC, id DESC`, with
default 20 and maximum 100. `before` and `before_id` are a required pair.
Supported CPO filters are `status`, `customer_id`, and `q`; `q` matches ticket
ID, subject, customer name, or email only inside the authorized CPO. Queue rows
never contain message bodies.

## Notifications and recovery

Customer create/reply notifies active eligible CPO support recipients, with an
active primary admin retained as recovery recipient. CPO reply/status change
notifies the owning customer's current email. The intent is inserted in the
same transaction as ticket truth; failure to persist it rolls back the mutation.
SMTP failure after commit is retried by the outbox and does not roll back the
ticket. Payloads contain CPO name, subject, status, time, and a configured
navigation URL, never the message body.

After an ambiguous mutation response, refetch ticket detail and retry only with
the same idempotency key. After reconnect, REST list/detail is authoritative;
there is no customer-support SSE. Invalid UUID/cursor/input returns 400;
unauthenticated callers receive 401; missing or foreign tickets receive 404;
missing CPO capability receives 403; an invalid status graph transition is 409.
