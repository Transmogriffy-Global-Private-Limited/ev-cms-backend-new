# SuperAdmin Support Desk

## Purpose and authority

The support desk is the platform's durable, cross-CPO conversation surface. It
lets a CPO start a support conversation and lets a SuperAdmin read the full
conversation, reply, and set its lifecycle status. It is not a tenant
impersonation mechanism, a general tenant-data browser, or a charger-control
surface.

This guide is the complete SuperAdmin workflow contract. The machine-readable
request and response shape is in
[`../../contracts/openapi/openapi.yaml`](../../contracts/openapi/openapi.yaml);
the manual authority classification is in
[`../../contracts/api/superadmin-permission-matrix.md`](../../contracts/api/superadmin-permission-matrix.md).

| Actor | Required context | Permitted support actions |
| --- | --- | --- |
| CPO staff | Current `CPO` bearer session and verified `X-CPO-App-ID` | Create, list, read, and reply only to tickets owned by that session's CPO |
| Platform SuperAdmin | Current `PLATFORM` bearer session | List, read, reply to, and set the status of every CPO ticket |
| Customer, anonymous caller, or a platform token with a CPO app ID | None is sufficient | No support endpoint is available |

There is currently one server-enforced platform authority: `PLATFORM`. The
support classifications in the permission matrix are frontend risk groupings,
not granular platform roles. The platform client must not send
`X-CPO-App-ID`, and it must not call the CPO support routes with a platform
token.

## Durable data model and ordering

A ticket is a durable parent record and every message is a durable immutable
child record. Creating a ticket and its initial CPO message happens in one
database transaction. Replying creates the message and updates the ticket in
one database transaction, so a successful reply response never represents an
acknowledged message without the corresponding ticket refresh.

| Field | Meaning and frontend use |
| --- | --- |
| `id` | Stable CMS UUID for the ticket. Use it as the route identifier and UI key. |
| `cpo_id` | The tenant that owns the conversation. It is present for platform review but is not a license to query unrelated tenant APIs. |
| `subject` | CPO-supplied, trimmed subject; 1-200 UTF-8 bytes after trimming. |
| `status` | `OPEN`, `PENDING`, `RESOLVED`, or `CLOSED`. See lifecycle semantics below. |
| `created_by_user_id` | UUID of the CPO staff identity that opened the ticket. No display name/email projection is returned. |
| `closed_at` | Present only after a platform `CLOSED` status update. It is not cleared by a later status update in the current implementation. |
| `created_at`, `updated_at` | RFC3339 timestamps. The queue is newest-first by `(updated_at DESC, id DESC)`. |
| `messages` | Full conversation, oldest-first by `(created_at ASC, id ASC)`. Every list item includes its complete message array. |
| `messages[].author_scope` | `CPO` or `PLATFORM`; use this to render speaker side. It is not a workflow owner indicator. |
| `messages[].author_user_id` | Author UUID only. The support contract does not currently return author profile data. |
| `messages[].body` | Trimmed free text; 1-10,000 UTF-8 bytes after trimming. Treat as untrusted user content. |

The current list endpoint has no pagination, filter, search, assignee, priority,
or summary projection. It returns all tickets visible to the caller and the full
message thread for each ticket. Use a loading state and do not add a UI claim
that the queue is paginated or that it is exhaustive after a locally applied
filter.

## Lifecycle: actual semantics, not inferred policy

The status values are deliberately small and do not encode an assignee, SLA,
priority, resolution reason, or the party currently expected to act.

```text
CPO creates ticket + first message
    -> OPEN

either CPO or PLATFORM appends a reply
    -> PENDING

PLATFORM explicitly selects OPEN, PENDING, RESOLVED, or CLOSED
    -> selected status
    -> CLOSED additionally records closed_at
```

Important implementation facts for the UI and support process:

- `PENDING` means only that the latest successful reply set this status. It
  does **not** tell the client whether the CPO or platform owes the next reply.
  Show last-message actor/time or an explicit UI-only work queue rule; do not
  call the field "waiting on CPO" or "waiting on platform".
- A platform user may set any of the four values directly. There is no enforced
  transition graph, reason field, or confirmation protocol in the API.
- `CLOSED` writes `closed_at`. Reopening to `OPEN`, `PENDING`, or `RESOLVED`
  does not clear the old `closed_at` value. Treat it as historical evidence of
  a prior closure, not as proof that the current status is closed.
- The server currently permits replies to `RESOLVED` and `CLOSED` tickets. Do
  not disable a reply control solely because of status unless the product later
  adds and documents that policy.
- Messages cannot currently be edited or deleted. A correction is a new reply.
- There is no ticket creation endpoint for platform staff. A platform response
  starts from an existing CPO-created conversation.

## SuperAdmin HTTP contract

All four platform operations require:

```http
Authorization: Bearer <platform-access-token>
Accept: application/json
```

Do not send a CPO app ID. The platform support API is not idempotency-keyed.

| Method and path | Success | Semantics |
| --- | --- | --- |
| `GET /api/v1/platform/support/tickets` | `200 SupportTicket[]` | Global cross-CPO queue, newest ticket update first; every item has its full message history. |
| `GET /api/v1/platform/support/tickets/{ticket_id}` | `200 SupportTicket` | Refresh one full conversation. |
| `POST /api/v1/platform/support/tickets/{ticket_id}/replies` | `200 SupportTicket` | Append one platform message and atomically set status to `PENDING`. |
| `PATCH /api/v1/platform/support/tickets/{ticket_id}/status` | `200 SupportTicket` | Set the status directly; `CLOSED` also records `closed_at`. |

`{ticket_id}` must be a non-nil UUID. A malformed or nil UUID is an
`invalid_request` error before service lookup.

### Reply

```http
POST /api/v1/platform/support/tickets/11111111-1111-4111-8111-111111111111/replies
Content-Type: application/json
Authorization: Bearer <platform-access-token>

{
  "body": "Please share the connector status and the time of the observed issue."
}
```

The server trims `body`, rejects an empty or over-10,000-byte result, creates a
message with `author_scope: "PLATFORM"`, updates the ticket timestamp, and
returns the refreshed thread. Disable repeat-click submission while the request
is outstanding. After a timeout or dropped response, first `GET` the ticket:
because there is no idempotency key, blindly retrying the POST can create a
second visible reply.

### Set status

```http
PATCH /api/v1/platform/support/tickets/11111111-1111-4111-8111-111111111111/status
Content-Type: application/json
Authorization: Bearer <platform-access-token>

{
  "status": "RESOLVED"
}
```

Accepted values are exactly `OPEN`, `PENDING`, `RESOLVED`, and `CLOSED` after
the server trims and uppercases the submitted value. Confirm a status change in
the UI because it changes durable shared workflow state. Refetch the returned
resource or apply the returned full ticket atomically; do not edit only a local
status badge while leaving the thread/timestamps stale.

### Representative response

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "cpo_id": "22222222-2222-4222-8222-222222222222",
  "subject": "Connector availability question",
  "status": "PENDING",
  "created_by_user_id": "33333333-3333-4333-8333-333333333333",
  "created_at": "2026-08-26T10:00:00Z",
  "updated_at": "2026-08-26T10:15:00Z",
  "messages": [
    {
      "id": "44444444-4444-4444-8444-444444444444",
      "ticket_id": "11111111-1111-4111-8111-111111111111",
      "author_user_id": "33333333-3333-4333-8333-333333333333",
      "author_scope": "CPO",
      "body": "The connector appears unavailable in our dashboard.",
      "created_at": "2026-08-26T10:00:00Z"
    },
    {
      "id": "55555555-5555-4555-8555-555555555555",
      "ticket_id": "11111111-1111-4111-8111-111111111111",
      "author_user_id": "66666666-6666-4666-8666-666666666666",
      "author_scope": "PLATFORM",
      "body": "Please share the connector status and the time of the observed issue.",
      "created_at": "2026-08-26T10:15:00Z"
    }
  ]
}
```

`closed_at` is omitted in this example because the ticket is not currently
closed. It is included as an RFC3339 timestamp when present.

## Counterpart CPO contract

The CPO UI is the source of new support conversations. These routes require
both a CPO bearer session and `X-CPO-App-ID`; they are tenant-scoped by the
authenticated session rather than a client-supplied CPO ID.

| Method and path | Success | CPO behavior |
| --- | --- | --- |
| `GET /api/v1/cpo/support` | `200 SupportTicket[]` | List only this CPO's complete conversations. |
| `POST /api/v1/cpo/support` | `201 SupportTicket` | Atomically create `OPEN` ticket plus first CPO message. Request requires trimmed `subject` and `body`. |
| `GET /api/v1/cpo/support/{ticket_id}` | `200 SupportTicket` | Fetch only an own-CPO ticket; another tenant's ID is not exposed. |
| `POST /api/v1/cpo/support/{ticket_id}/replies` | `200 SupportTicket` | Append a CPO message and set `PENDING`. |

The CPO status and platform response are discovered by later GET refreshes.
There is no support-specific SSE, platform notification, email, or webhook
delivery in this feature. A SuperAdmin frontend must not promise real-time
ticket arrival or automatic customer/CPO notification.

## Frontend implementation workflow

1. On opening the Support screen, request the global queue. Render it by
   server order and show `subject`, status, tenant identifier or separately
   authorized CPO display data, last update, last message actor, and message
   count. The ticket response itself does not include CPO business name or
   author display profiles.
2. On selection, render `messages` in the supplied oldest-first order. Render
   message text as text, not trusted HTML. Do not place it in analytics,
   URLs, logs, error reporting, or client-side diagnostics.
3. Send one reply at a time. Use the returned full ticket to replace the local
   cached ticket, and refresh the queue to restore the server sort order.
4. Present status selection as a durable workflow action with confirmation.
   Do not infer who is waiting from `PENDING`; use last actor and an explicit
   local process label if product policy requires it.
5. On any reconnect, page revisit, successful mutation, or ambiguous mutation
   outcome, reload authoritative REST state. There is no ticket SSE/replay or
   version token for conflict detection.
6. If two users update the same ticket concurrently, the service has no
   optimistic-concurrency revision. The last committed update determines the
   stored status/updated timestamp. Show the refreshed server response instead
   of treating local state as authoritative.

## Errors, retries, and recovery

| Condition | HTTP result | Safe client behavior |
| --- | --- | --- |
| Missing/invalid platform authentication | `401` from authentication middleware | Clear/refresh the platform session through the normal auth flow; do not retry with a CPO token. |
| Authenticated non-platform caller | `403 forbidden` | Treat as an authorization boundary, not a retryable support failure. |
| Invalid UUID, malformed JSON, blank-after-trim text, over-limit text, or invalid status | `400 invalid_request` | Keep the draft locally, show field-level guidance, and correct before resubmitting. |
| Ticket absent, or CPO caller asks for another tenant's ticket | `404 support_ticket_not_found` | Refresh the queue; do not reveal whether an inaccessible tenant ticket exists. |
| Unexpected persistence failure | `500 internal_error` | Show a safe retry/support message and retain only a user-owned draft. Capture `X-Request-ID` without recording ticket text or credentials. |
| Network timeout/aborted request | No reliable outcome | GET the ticket/queue first. Never automatically retry reply POST because it can duplicate text. Status PATCH also needs a GET refresh before another deliberate action. |

The server does not expose a `Retry-After`, an idempotency key, ticket version,
or message deduplication key for this workflow. GET operations are safe to
retry. Mutation retries require an informed user decision after authoritative
state is read.

## Security, privacy, and operational limits

- Ticket content, CPO ID, and author IDs are support data. Limit the screen to
  authorized platform staff and never use raw message bodies in telemetry,
  crash reporting, application logs, search indices, or client URLs.
- The support desk is deliberately narrower than tenant support access. It does
  not grant CPO session impersonation, wallet access, customer credentials,
  billing operations, charger commands, HAL data-plane access, or encrypted
  integration-secret plaintext.
- There are no attachments, rich-text sanitization service, ticket assignment,
  priority, tags, SLA timers, status reason, bulk update, archive, deletion,
  export, or platform-created ticket endpoint.
- There are no push notifications, email delivery, webhooks, realtime stream,
  audit-log contract, or automatic escalation specific to support tickets.
- The database retains the durable ticket/message records subject to the
  repository's broader data-retention policy; this feature has no separate
  support-ticket retention or purge API.

## Verification and authority map

For a frontend integration, validate at least these cases against a safe test
environment:

1. A CPO creates a ticket and sees its initial `CPO` message in the response.
2. A platform user lists and reads that full conversation; a different CPO
   cannot read it.
3. A platform reply appears as `PLATFORM`, updates `updated_at`, and yields
   `PENDING` without claiming a waiting party.
4. `CLOSED` returns `closed_at`; a subsequent explicit `OPEN` shows the known
   retained `closed_at` behavior.
5. Blank, oversized, malformed, cross-tenant, unauthorized, and ambiguous
   network cases follow the error/recovery rules above.

The canonical backend route and schema sources are the OpenAPI document and
the permission matrix linked at the beginning of this guide. This workflow is
implemented as durable REST state; it has no live deployment assertion of its
own.
