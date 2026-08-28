# SuperAdmin Support Desk

## Purpose and authority

The support desk is the durable, tenant-scoped conversation surface between CPO
staff and Platform SuperAdmins. PostgreSQL ticket, message, and lifecycle-event
rows are authoritative. This core workflow has no support SSE, email, webhook,
or platform-notification delivery.

The machine-readable HTTP contract is
[`../../contracts/openapi/openapi.yaml`](../../contracts/openapi/openapi.yaml).
The SuperAdmin operation classification remains in
[`../../contracts/api/superadmin-permission-matrix.md`](../../contracts/api/superadmin-permission-matrix.md).

| Actor | Required context | Allowed support actions |
| --- | --- | --- |
| CPO staff | Current CPO bearer session, verified `X-CPO-App-ID`, and the relevant current capability (`support.create`, `support.read`, or `support.reply`) | Create, list/read, or reply only to tickets owned by the trusted CPO scope |
| Platform SuperAdmin | Current `PLATFORM` bearer session | List/filter every ticket, read detail, reply, and change ticket status |
| Customer or anonymous caller | None is sufficient | No support API is available |

Ticket content, CPO IDs, and author IDs are support data. Render message text as
untrusted text and never put it in URLs, telemetry, logs, or crash reports.

## Durable records and mutation order

A ticket owns immutable messages and immutable lifecycle events. Detail returns
both arrays oldest-first by `(created_at, id)`; queue list never returns message
bodies or lifecycle events.

```text
create -> ticket + initial CPO message + CREATED event, one transaction
reply  -> lock/re-read -> message + MESSAGE_ADDED event + projection, one transaction
status -> lock/re-read -> validate graph -> projection + STATUS_CHANGED event, one transaction
```

Lifecycle events retain actor scope/user, timestamp, optional bounded reason,
and (for every actual status change) the previous and next status. No endpoint
can alter a ticket's status without appending that immutable history.

## Status lifecycle

The status set is `OPEN`, `IN_PROGRESS`, `RESOLVED`, and `CLOSED`. Migration
000057 changed historical `PENDING` rows to `IN_PROGRESS`; `PENDING` is not an
accepted API value.

```text
OPEN        -> IN_PROGRESS | RESOLVED | CLOSED
IN_PROGRESS -> RESOLVED | CLOSED
RESOLVED    -> OPEN | CLOSED
CLOSED      -> OPEN
```

Submitting the current status is a side-effect-free retry. Any other transition
outside this graph is `409 invalid_support_transition`. Only platform callers
may change status explicitly. `CLOSED` writes `closed_at`; every non-closed
transition clears it. A CPO reply to `RESOLVED` or `CLOSED` automatically
reopens to `OPEN`, clears `closed_at`, and records both `MESSAGE_ADDED` and a
separate `STATUS_CHANGED` event. A platform reply does not change status.

## HTTP contract

All request JSON must be exactly one object no larger than 32 KiB. Unknown
fields, multiple JSON values, arrays, `null`, malformed JSON, blank-after-trim
text, over-limit values, and malformed/nil UUIDs are rejected safely.

### Queue list

`GET /api/v1/platform/support/tickets` is the global queue. `GET
/api/v1/cpo/support` is forced to the caller's trusted CPO. Both respond:

```json
{
  "tickets": [{
    "id": "ticket UUID",
    "cpo_id": "tenant UUID",
    "cpo_name": "Human CPO business name",
    "subject": "Connector availability",
    "status": "IN_PROGRESS",
    "created_at": "2026-08-26T10:00:00Z",
    "updated_at": "2026-08-26T10:15:00Z",
    "message_count": 2,
    "last_message_at": "2026-08-26T10:15:00Z",
    "last_message_scope": "PLATFORM"
  }],
  "next_before": "2026-08-26T10:15:00Z",
  "next_before_id": "ticket UUID",
  "has_more": true
}
```

The order is `(updated_at DESC, id DESC)`. `limit` defaults to 20 and is 1–100.
Pass both `before` and `before_id` from a prior page; supplying only one returns
`400 invalid_cursor`. Both lists accept `status` and case-insensitive bounded
`q` (subject or ticket UUID). The platform list also accepts `cpo_id`; CPO
callers can never expand trusted tenant scope using that query parameter.

### Detail, reply, and status

`GET /api/v1/.../support/{ticket_id}` returns the current ticket, `messages`,
and lifecycle `events`. Cross-CPO detail remains `404 support_ticket_not_found`.

`POST /api/v1/.../support/{ticket_id}/replies` accepts a trimmed 1–10,000-byte
`body` and optional trimmed `idempotency_key` (maximum 120 bytes). Reusing a
key for the same ticket returns current detail without another reply. The
durable event-key constraint and ticket row lock protect concurrent retries.

`PATCH /api/v1/platform/support/tickets/{ticket_id}/status` accepts a
case-normalized status and optional 500-byte `reason`; the reason is retained
in transition history. Use returned detail after every mutation. On an
ambiguous reply retry, reuse the same key only for the same message; on an
ambiguous status request, refresh detail before making another deliberate call.

## Errors, limits, and future work

| Condition | Result | Safe client action |
| --- | --- | --- |
| Invalid request/query | `400` with stable safe code | Preserve local draft and correct it. |
| Missing or cross-CPO ticket | `404 support_ticket_not_found` | Refresh authorized queue; do not infer tenant existence. |
| Unsupported transition | `409 invalid_support_transition` | Refresh detail and choose a graph edge. |
| Wrong authority/capability | `403 forbidden` | Treat as an authorization boundary. |
| Persistence failure | `500 internal_error` | Keep user draft and refresh durable detail. |

There is no support mail, notification, assignment, priority, attachment,
rich text, deletion, bulk update, platform-created ticket, or realtime feed in
this slice. Future delivery must consume committed support facts and must not
make lifecycle truth depend on SMTP or another notification channel.

## Verification

Verify capability/tenant isolation, every graph edge and invalid edge, created
and status history, automatic reopen with `closed_at` clearing, reply
idempotency, cursor/filter behavior, bounded summaries, and strict JSON.
Full PostgreSQL mutation and concurrent-writer proof needs a separately selected
disposable `TEST_DATABASE_URL`; never run it against a live database.
