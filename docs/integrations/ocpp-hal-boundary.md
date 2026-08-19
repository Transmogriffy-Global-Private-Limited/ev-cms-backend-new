# CMS and OCPP HAL V1 Boundary

## Scope and Authority

The CMS consumes the v1 contract from the read-only HAL provider reference
`ocpp-hal-go-new` commit `21836e5d98967399d599d6afeca52fe1c375ec0d`.
Neither service shares a database.

CMS owns customer/CPO scope, tariff selection, GST, wallet holds and settlement,
and the customer REST projection. HAL owns OCPP connection/reconnection state,
charger commands, exact OCPP transaction IDs, meter facts, and the charger
protocol lifecycle. A RemoteStart or RemoteStop acknowledgement is command
evidence only; charger-originated StartTransaction and StopTransaction facts
establish start and completion truth.

## Implemented CMS Boundary

- `src/halclient` sends authenticated mapping, start, and stop requests to the
  versioned HAL REST boundary with bearer, correlation, idempotency, timeout,
  and typed HTTP errors.
- `POST /v1/hal-facts` uses a distinct HAL-to-CMS bearer. It requires
  `Idempotency-Key` to equal `fact_id`, verifies RFC 8785 JCS SHA-256 immutable
  content, records a durable receipt, accepts exact duplicates once, and rejects
  altered fact-ID reuse.
- CMS persists start intents, wallet holds, command records, mappings, fact
  receipts, and durable connection/connector runtime projections in migration
  `000028_cms_hal_charging_vertical`.
- User App routes poll only CMS data. They never synchronously call HAL for a
  map, session, or meter read.
- `src/halops` is the CMS integration capability above `halclient`: it owns
  mapping synchronization, exact CMS command identity reconciliation, and the
  independent fact ingress socket. Business services authorize first and never
  build raw HAL requests.
- `src/liveops` owns current CMS-projection reads for charger, connector,
  session, and fleet state. It centralizes freshness: connection evidence uses
  `HAL_V1_CONNECTION_STALE_AFTER`, meter evidence uses the independent
  `HAL_V1_METER_STALE_AFTER`, and connector availability is never fresh when
  parent connection evidence is stale, offline, or unknown.
- Durable `operational_events` are written in the same transaction as an
  accepted fact projection. They are notification-sized; REST snapshots remain
  the authority after missed delivery or reconnect.

## Customer Flow

```text
customer bearer + CPO app ID
-> CMS synchronizes the exact committed charger/connector mapping as an operational prerequisite
-> CMS locks wallet and resolves User Group > charger > hub tariff
-> CMS freezes the commercial tariff and independent hub GST, derives integer Wh affordability, creates hold,
   start intent, one-use appv1_ credential hash, and command identity
-> CMS reconfirms the mapping after the commercial transaction to close an inventory-change race
-> CMS requests HAL start
-> HAL fact transaction.started materializes one CMS ACTIVE session
-> HAL meter/connection/status facts update CMS projections
-> customer stop persists one CMS stop command and asks HAL
-> HAL transaction.completed settles the wallet once from exact final Wh
```

The raw credential is sent to HAL only for the start command; CMS retains only
its SHA-256 value. It is intentionally neither persisted nor replayed: a lost
or absent HAL command is terminalized and a later customer retry creates a new
credential and CMS command identity. Meter values remain integer Wh and are never interpolated.
The v1 tariff contains energy and idle-fee commercial components; GST belongs
to the hub tax snapshot. No fixed or
time component is invented by this implementation.

## Configuration and Security

`HAL_V1_BASE_URL`, `HAL_V1_CMS_BEARER_TOKEN`, and
`HAL_V1_CMS_FACT_BEARER_TOKEN` configure the two directions. When a base URL is
set both tokens are required; when no base URL is supplied customer charging
returns `hal_unavailable`. Local topology uses loopback endpoints only.

HAL emits a new ordered `charger.connection.updated` fact for initial
connection and for accepted current-connection Heartbeats. CMS applies only an
advancing `connection_sequence`, so a delayed fact cannot change state or renew
`observed_at`. Realtime is an invalidation hint only: User App and CPO refreshes
reconstruct the same connection state/freshness from these durable projections.

No token, raw credential, authorization header, or fact body is logged. The
HAL client does not retry a mutating command with a new identity. A caller
reconciles the same command identity after a transport ambiguity.

## Start-command failure and reconciliation states

`RECONCILIATION_REQUIRED` is active only while the outcome of an already
attempted `RequestStart` is unknown. It is not a generic HAL-error state.

```text
mapping prerequisite fails before a commercial start is committed
    -> 503 charger_mapping_unavailable
    -> no intent, hold, or command record

post-commit mapping confirmation fails before RequestStart
    -> atomically RELEASED hold + REJECTED/EXPIRED intent + NOT_ATTEMPTED command
    -> 503 charger_mapping_unavailable

RequestStart invoked but transport/provider outcome is uncertain
    -> RECONCILIATION_REQUIRED start intent and command; HELD hold
    -> exact GET by the original cms_command_id

exact GET finds command
    -> project the matching HAL command state; do not release the hold

exact GET returns canonical HTTP 404
    -> atomically CONFIRMED_ABSENT command + RELEASED hold + REJECTED intent
    -> EXPIRED instead when command_expires_at has passed
    -> the connector is no longer blocked; a later customer request is fresh

exact GET has 401/403/409/422/5xx, timeout, malformed, or other failure
    -> retain RECONCILIATION_REQUIRED and HELD; record safe lookup diagnostics
```

Confirmed absence is safe only because this is HAL's durable exact command
lookup. CMS locks the command, intent, and hold; verifies the expected active
states and that no materialized session or `charging_sessions` row exists; then
releases only a `HELD` hold in the same transaction. A concurrent
`transaction.started` fact locks the same intent and wins without releasing a
hold. The cleanup is idempotent and creates no wallet ledger, debit, capture,
or settlement.

START and STOP deliberately differ: a missing STOP command never proves that a
materialized session stopped, so its command remains reconciliation-required
and session settlement continues to require `transaction.completed`.

## Post-deployment Connection-Liveness Acceptance

This procedure is not evidence until it is run after separately approved
deployment. Use only the mapped development charger `bd9099` / connector `1`;
do not edit either service database manually.

1. Restart the new HAL. Confirm it projects the prior runtime as `UNKNOWN`,
   resets its process-scoped generation baseline, and delivers the resulting
   connection fact to CMS.
2. Connect `cpconsole` once with `-url wss://dev-ocpphal-new.transev.site`,
   `-id bd9099`, and `-connector 1`. Do not append the identity to `-url`.
3. Confirm HAL runtime is `ONLINE`, generation `1`, and has a higher durable
   connection sequence. Confirm `charger.connection.updated` is delivered with
   HTTP `204`, then confirm CMS REST reports `ONLINE`/`FRESH`.
4. Send `StatusNotification(Available)` for connector `1`; confirm its fact is
   delivered with `204` and CMS REST reports connector `AVAILABLE`/`FRESH`.
5. Keep the charger connected past the former short meter threshold, send a
   Heartbeat, and confirm HAL `last_observed_at` and connection sequence advance
   without changing generation. Confirm the renewed fact reaches CMS, then
   refresh the User App and confirm its REST reconstruction remains
   `ONLINE`/`FRESH` and `AVAILABLE`/`FRESH`.
6. Keep liveness flowing through at least one complete configured heartbeat
   cycle. Confirm no false stale transition occurs. Then terminate `cpconsole`;
   confirm HAL `OFFLINE`, a higher ordered fact with HTTP `204`, and CMS/User
   App unavailable/stale state.
7. Separately test silence without a disconnect callback. After
   `HAL_V1_CONNECTION_STALE_AFTER` expires, CMS must treat historical `ONLINE`
   and connector `Available` as stale/unavailable without synthesizing an
   `OFFLINE` fact.

## Current Limitations and Required Follow-up

The durable data and route surfaces are in source, including bounded
reconciliation for pending/reconciliation-required mappings and exact command
identity lookup. CPO charger creation, edits, and status changes leave a
durable mapping record and attempt provider synchronization only after their
CMS transaction commits. Provider failure remains a visible pending state; it
never fabricates command/session outcome.

Full Postgres-to-HAL-to-virtual-charger acceptance has not yet been run because
the required disposable CMS/HAL databases and virtual charger topology were not
configured in this slice. Do not claim physical charger acceptance, restart
recovery, or full vertical-slice completion until the planned dual-service tests
prove them.
