# Platform Realtime Event Contract

## Purpose and Authority

Platform realtime keeps superadmin views current without turning a connection
or browser into a source of truth.

PostgreSQL `platform_events` rows are the durable event source. REST resources
remain authoritative. A client receiving an event refreshes the affected REST
resource when it needs current state.

## Endpoints

- `GET /api/v1/platform/events` provides cursor-based catch-up and polling.
- `GET /api/v1/platform/realtime/stream` provides an authenticated
  `text/event-stream`.

Both require a current `PLATFORM` bearer session. CPO and customer sessions are
rejected. Tokens must be sent in the `Authorization` header and must never be
placed in a query string.

Browser clients should use `fetch()` streaming because native `EventSource`
does not provide the required bearer-header flow.

## Event Envelope

```json
{
  "id": 14582,
  "type": "platform.cpo.activated",
  "actor_user_id": "2ecdcf2d-afbc-4629-8707-d66b4f973ea7",
  "resource_type": "CPO",
  "resource_id": "76b5a7ce-cd7d-435a-b090-e758bce3b6ef",
  "data": {
    "status": "ACTIVE"
  },
  "occurred_at": "2026-07-24T12:14:52Z"
}
```

Required fields:

- `id`: globally increasing durable event cursor;
- `type`: canonical semantic fact name;
- `resource_type`: affected resource class;
- `data`: safe event-specific object;
- `occurred_at`: committed fact time.

Optional fields:

- `actor_user_id`: authenticated platform actor when the fact was user-driven;
- `resource_id`: affected resource identifier.

Events must never include OTPs, temporary passwords, tokens, mail plaintext,
SMTP credentials, Razorpay secrets, cryptographic keys, or unnecessary tenant
PII.

## SSE Framing

```text
id: 14582
event: platform.cpo.activated
data: {"id":14582,"type":"platform.cpo.activated",...}

```

Heartbeat comments are sent at
`PLATFORM_REALTIME_HEARTBEAT_INTERVAL`:

```text
: heartbeat 2026-07-24T12:15:00Z

```

The heartbeat revalidates the encrypted token, durable session, active user,
and platform authority. The server closes the stream if validation fails.

## Ordering, Duplicates, and Recovery

- Delivery is at least once.
- Events are replayed in ascending `id` order.
- Clients deduplicate using `id`.
- `Last-Event-ID` is preferred on reconnect; `after_id` is the explicit
  fallback.
- A REST catch-up page returns `next_cursor` and `has_more`.
- Events older than `PLATFORM_EVENT_RETENTION` are deleted by the durable
  platform-maintenance worker.
- A cursor older than the earliest retained event receives
  `409 realtime_cursor_expired`.

After cursor expiry the client:

1. reloads platform overview and any visible authoritative REST resources;
2. reconnects without the expired cursor;
3. resumes processing new events.

## Production and Consumption

An event-producing command must write the state change, audit record, event,
and applicable notification work in the same PostgreSQL transaction. The event
cannot announce uncommitted or rolled-back state.

Current producers:

- CPO creation;
- CPO business-profile replacement;
- CPO activation and suspension;
- CPO app-ID rotation;
- CPO primary-administrator replacement/restoration;
- CPO primary-administrator onboarding resend;
- CPO administrative-session revocation;
- manual subscription-plan creation, draft changes, publication, and archive;
- manual CPO subscription issue, renewal, plan change, and status transition;
- manual CPO entitlement-override changes;
- worker heartbeat records provide worker-state source data.
- readiness requires at least one fresh, healthy instance for each required
  worker name; stale records from replaced instances remain observable but do
  not make a healthy replacement unavailable.

Planned producers are registered in
`docs/plans/superadmin-control-plane.md`.

Current CPO event names and safe payload meanings:

| Event | Data | REST refresh |
| --- | --- | --- |
| `platform.cpo.created` | CPO ID/status/app-ID mode | collection |
| `platform.cpo.profile_updated` | empty object; refetch the resource | collection and CPO detail |
| `platform.cpo.activated` / `platform.cpo.suspended` | previous status, status, reason | collection and CPO detail |
| `platform.cpo.app_id_rotated` | current app-ID mode; app ID is refetched | CPO detail |
| `platform.cpo.primary_admin_changed` | previous/current user IDs and reason | primary-admin resource |
| `platform.cpo.primary_admin_onboarding_resent` | primary user ID and reason | primary-admin resource |
| `platform.cpo.admin_sessions_revoked` | reason and revoked counts | audit/recovery display |
| `platform.subscription.plan_created` / `plan_draft_updated` / `plan_published` / `plan_archived` | code or version metadata | plan catalog/detail |
| `platform.subscription.issued`, `platform.subscription.renewed`, `platform.subscription.plan_changed`, `platform.subscription.activated`, `platform.subscription.paused`, `platform.subscription.resumed`, `platform.subscription.past_due`, `platform.subscription.expired`, `platform.subscription.cancelled` | CPO ID, status, plan version ID | CPO subscription/detail |
| `platform.subscription.entitlement_override_set` / `entitlement_override_removed` | feature key | CPO entitlements |

Lifecycle retries that request the already-current state do not create another
lifecycle event. Explicit session revocation does create an event even when the
counts are zero because the operator command itself is auditable evidence.

## Connection Behavior

- Polling interval: `PLATFORM_REALTIME_POLL_INTERVAL`.
- Per-query batch: `PLATFORM_REALTIME_BATCH_SIZE`.
- Heartbeat interval: `PLATFORM_REALTIME_HEARTBEAT_INTERVAL`.
- Slow or disconnected clients reconnect and replay; no per-connection
  in-memory backlog is authoritative.
- The stream does not accept client commands.
- REST commands remain independently retryable and auditable.

## Verification

- route authentication rejects missing, CPO, and customer sessions;
- OpenAPI and runtime route sets match;
- PostgreSQL lifecycle tests cover ordered emission, cursor recovery, expiry,
  and worker heartbeat state;
- stream tests cover SSE framing, heartbeat, reconnect, and revoked-session
  termination;
- full verification is defined in repository `AGENTS.md`.
