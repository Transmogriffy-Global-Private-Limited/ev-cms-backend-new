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
| CPO `chargers.operations` | `GET /api/v1/cpo/operations/events` | `GET /api/v1/cpo/operations/realtime/stream` | Authenticated active CPO membership, matching `X-CPO-App-ID`, and fresh `chargers.operations` |
| CPO `chargers.operations` live-session table | `GET /api/v1/cpo/operations/live-sessions/snapshot` | `GET /api/v1/cpo/operations/live-sessions` | Authenticated active CPO membership, matching app ID, and fresh `chargers.operations`; full initial/replacement `LiveChargingSessionListResponse` snapshots only |
| Platform | `GET /api/v1/platform/cpos/{cpo_id}/operations/events` | `GET /api/v1/platform/cpos/{cpo_id}/operations/realtime/stream` | `PLATFORM`, selected existing CPO, observation only |
| User App legacy/general feed | `GET /api/v1/app/operations/events` | `GET /api/v1/app/operations/realtime/stream` | Authenticated CPO-local customer and matching app ID; retained invalidation/cursor compatibility only |
| User App live-session collection | `GET /api/v1/app/operations/live-sessions/snapshot` | `GET /api/v1/app/operations/live-sessions` | Authenticated customer/app scope; full initial/replacement `CustomerLiveChargingSessionListResponse` only |
| User App selected charger | Existing `GET /api/v1/app/chargers/{charger_id}` | `GET /api/v1/app/operations/charger-availability?charger_id={public_id}` | Authenticated customer/app scope and current charger visibility; full initial/replacement `CustomerCharger` only |
| User App charger-card batch | `GET /api/v1/app/operations/charger-chargeability?charger_ids=...` | `GET /api/v1/app/operations/charger-chargeability/stream?charger_ids=...` | Authenticated customer/app scope; 1–100 public IDs, full initial/replacement compact `CustomerChargeabilityResponse` only |

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

The primary CPO live-session stream deliberately does not expose the durable
event log to the frontend. It establishes the latest committed
`CHARGING_SESSION` event watermark **before** reading the current CMS snapshot,
then sends `event: snapshot`. Later committed `charging.session_changed`,
`charging.meter_changed`, or `charging.telemetry_changed` records cause one
`event: live_sessions` replacement snapshot. A post-watermark commit may cause
a redundant projection refresh; it cannot be skipped between snapshot and
event consumption. The data in both frames is the full
`LiveChargingSessionListResponse`, including removal after completion. A
reconnect always gets a current snapshot; JSON recovery and keyset pagination
use `/api/v1/cpo/operations/live-sessions/snapshot`. The retained
`/live-sessions/events` cursor route is advanced reconciliation tooling, not
required for the normal UI. The deprecated `/live-sessions/realtime/stream`
route is a compatibility alias. Both SSE aliases recheck active CPO session,
matching app ID, and `chargers.operations` on heartbeat and
`X-CPO-App-ID` at each heartbeat.

The dedicated User App streams likewise keep operational events internal:
events only wake a committed CMS re-projection and are never sent as their
browser state. `live-sessions` uses a customer-scoped collection: its snapshot
and replacement frames contain all current owned materialized sessions, and a
customer can validly have more than one. Its wake-up filter includes owned
session facts plus tenant-shared charger/connector facts for the collection's
current resources. `charger-availability` is scoped to one already authorized
public charger ID and includes the complete current `CustomerCharger` object,
including customer-specific chargeability. `charger-chargeability` is the
corresponding one-stream batch projection for card UIs; it accepts only public
IDs, omits no-longer-visible IDs, and replaces the complete requested set on
every semantic change. Because chargeability also depends on wallet, holds,
commercial admission, tariff/GST, mapping, administrative lifecycle, durable
occupancy, and freshness, these charger streams watch the retained CPO
operational wake-up set rather than treating OCPP status as the sole trigger.
Both establish a watermark before their initial snapshot, revalidate on
heartbeat, and periodically reproject so time-derived freshness or
chargeability cannot remain stale when no fact arrives. Fingerprints suppress
replacement frames when the projection is unchanged. Reconnect begins with
current state rather than event replay. The generic User App feed remains
unchanged for compatibility.

`can_charge` consumes the existing committed `liveops` predicates and does not
repair or reinterpret their operational `OFFLINE`, `UNKNOWN`, or `STALE`
semantics. Its reason vocabulary preserves that distinction: `CHARGER_OFFLINE`
means the materialized parent connection says offline, while unknown or stale
live evidence returns the corresponding `*_STATE_UNKNOWN` or `*_STALE` reason.

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
