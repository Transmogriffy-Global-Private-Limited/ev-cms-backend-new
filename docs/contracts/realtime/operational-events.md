# CMS Operational Events

## Purpose and Authority

Operational events are durable, scope-filtered invalidation hints written in
the same CMS transaction that accepts a newer HAL-derived projection fact.
They never contain HAL credentials, raw provider payloads, OCPP transaction
identifiers, or financial truth. The corresponding REST projection is always
authoritative after reconnect, duplicate delivery, or a missed event.

For `resource_type=CHARGING_SESSION`, `resource_id` is always the materialized
CMS `charging_sessions.id`, not the originating `cms_start_intent_id`. A User
App consumer can therefore refetch it directly through
`GET /api/v1/app/charging-sessions/{resource_id}`. Meter events whose sequence
is no longer the stored committed session sequence are suppressed rather than
emitted as misleading invalidations.

## Scoped Recovery and Streams

| Consumer | REST recovery | SSE stream | Scope |
| --- | --- | --- | --- |
| CPO ADMIN | `GET /api/v1/cpo/operations/events` | `GET /api/v1/cpo/operations/realtime/stream` | Authenticated tenant and matching `X-CPO-App-ID` |
| CPO ADMIN live-session table | `GET /api/v1/cpo/operations/live-sessions/events` | `GET /api/v1/cpo/operations/live-sessions/realtime/stream` | Authenticated CPO ADMIN and matching app ID; `CHARGING_SESSION` events only; refresh `/operations/live-sessions` |
| Platform | `GET /api/v1/platform/cpos/{cpo_id}/operations/events` | `GET /api/v1/platform/cpos/{cpo_id}/operations/realtime/stream` | `PLATFORM`, selected existing CPO, observation only |
| User App | `GET /api/v1/app/operations/events` | `GET /api/v1/app/operations/realtime/stream` | Authenticated CPO-local customer and matching app ID |

REST accepts `after_id` and optional `limit` (1–500; default 100). SSE accepts
the same cursor or `Last-Event-ID` when the query parameter is absent. Records
are ordered by increasing ID; delivery is at least once, so clients deduplicate
the numeric ID and persist only their most recently handled ID.

SSE is bearer-authenticated. Clients use `fetch()` streaming because native
`EventSource` cannot set the bearer or CPO app-ID headers. Streams poll the
durable table, emit `id`, `event`, and JSON `data` frames, heartbeat at the
configured platform realtime interval, and revalidate the durable session at
each heartbeat. A revoked, expired, scope-changed, or CPO-mismatched session
causes stream closure.

The CPO live-session pair deliberately has its own cursor. It filters the
durable log to `resource_type=CHARGING_SESSION` and the committed
`charging.session_changed`, `charging.meter_changed`, and
`charging.telemetry_changed` event types. It is an invalidation feed only:
the paged `GET /api/v1/cpo/operations/live-sessions` CMS snapshot remains the
state authority, including removal after completion. Its stream additionally
checks the current CPO ADMIN role and `X-CPO-App-ID` at each heartbeat.

## Event Shape

```json
{
  "id": 42,
  "cpo_id": "uuid",
  "type": "charging.meter_changed",
  "resource_type": "CHARGING_SESSION",
  "resource_id": "uuid",
  "data": {},
  "occurred_at": "2026-08-12T00:00:00Z"
}
```

`customer_id` is present only on customer-scoped stored records. CPO consumers
receive their tenant's safe records; a customer receives its own records plus
safe charger/connector availability changes for the tenant. Current event
types are `charging.command_changed`, `charger.live_state_changed`,
`connector.live_state_changed`, `charging.session_changed`, and
`charging.meter_changed`, plus `charging.telemetry_changed`. The latter is
emitted only after a newer accepted `transaction.soc` projection and uses the
same materialized-session resource identity and REST recovery path, so a
SoC-only update invalidates the live detail without depending on
`meter_sequence`.

## Producer, Deduplication, and Recovery

The shared HAL fact ingestor validates the service bearer, schema version,
canonical digest, and `fact_id` before calling the CMS projection socket. A
fact receipt and projection change share one transaction. Exact fact replay is
a no-op; stale connection, connector, and meter sequences do not create an
operational event. A stream is not a command channel and does not invoke HAL.

On reconnect or a gap, read the scoped REST event page, deduplicate records,
then refresh `GET` fleet/charger/session resources. Event retention uses
`PLATFORM_EVENT_RETENTION`; expiry is expected and never makes a client state
authoritative.
