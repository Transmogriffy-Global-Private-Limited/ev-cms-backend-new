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

For an HTTP-originated mapping, start, or stop mutation, CMS `RequestLogger`
generates the canonical request ID, stores it in Gin context, and the charging
handler forwards that value as `X-Correlation-ID`. The User App does not need
to send `X-Request-ID`; a client value is not the CMS-to-HAL correlation
authority. The CMS HAL client rejects an empty correlation ID before sending a
mutation, while non-mutating exact-command lookup remains correlation-free.

## HAL command-response contract

## Independent customer limits and wallet safety

The customer-selected `AUTO`, `ENERGY`, `TIME`, or `MONEY` classification is
immutable intent, not HAL's stop cause and not tariff semantics. CMS separately
derives a wallet-safety bound only in the tariff billing dimension: Wh for an
energy tariff, seconds for a time tariff, and a fixed admission-only charge for
a session tariff. It never predicts energy from duration, duration from energy,
or either from charger capacity.

The authenticated start command may therefore carry both
`energy_limit_wh`/`energy_limit_source` and
`max_duration_seconds`/`duration_limit_source`. Each source is `NONE`,
`CUSTOMER_ENERGY`, `CUSTOMER_TIME`, `CUSTOMER_MONEY`, or `WALLET`. HAL persists
these facts without interpreting tariff, GST, wallet, buffer, or final billing.
Its meter/deadline stop workflow reports `ENERGY_LIMIT`, `TIME_LIMIT`,
`MONEY_LIMIT`, or `WALLET_LIMIT` from the specific threshold source. Final CMS
settlement remains based on actual completion evidence and the frozen tariff
and tax snapshots; a finite wallet buffer is held once for metered-stop
overshoot and never changes final price calculation.

## Optional charger SoC telemetry

HAL may send additive `transaction.soc` immutable facts for valid OCPP 1.6
`SoC` MeterValues evidence. The payload contains canonical decimal-string
`soc_percent` in the inclusive `0`–`100` range, its `soc_observed_at`, and an
independent increasing `soc_sequence`, together with existing transaction,
start-intent, charger, and connector identities. SoC-only facts are valid and
do not contain invented energy; energy-only `transaction.meter` facts do not
repeat cached SoC.

CMS stores the first accepted SoC once and advances latest SoC only with a
newer SoC sequence and non-regressive observation time. It preserves that last
observation after completion. Missing SoC remains NULL/unknown: it is never
derived from energy, battery capacity, completion, or a default of zero. Live
SoC freshness uses the established MeterValues stale horizon but is computed
independently from energy-meter freshness; an offline/stale parent connection
leaves retained SoC historical rather than fresh.

CMS accepts a successful HAL start, stop, or exact-command lookup only when it
contains a `command` object with exact snake_case `hal_command_id`,
`cms_command_id`, `kind`, `state`, `hal_transaction_id`,
`ocpp_transaction_id`, and `updated_at` fields. HAL/CMS command identities
must be nonzero UUIDs, the returned CMS ID must be the requested ID, kind and
durable state must be supported, and a present transaction ID must not be the
zero UUID. A missing wrapper, Go-named fields, zero UUID, stale/mismatched CMS
identity, or unsupported kind/state is an integration contract failure, not a
successful delivery; CMS leaves the durable HAL identity unknown and enters its
normal exact-ID reconciliation path.

`NULL` means a HAL identity is not yet known. The all-zero UUID is never a
valid identity. When authoritative `transaction.started` evidence supplies a
nonzero HAL command ID, CMS fills a NULL or historical zero value and proceeds
through the normal locked materializer. Only different nonzero recorded and
authoritative IDs return `409 hal_start_evidence_conflict`.

## Start-command failure and reconciliation states

`RECONCILIATION_REQUIRED` is active only while the outcome of an already
attempted `RequestStart` is unknown. It is not a generic HAL-error state.

HAL fact ingress is independently authenticated and durable. Expected fact
rejections return stable 4xx/409 codes (`invalid_hal_fact`,
`unsupported_hal_fact`, `fact_integrity_violation`, or
`hal_start_evidence_conflict`) so HAL records terminal reconciliation rather
than retrying a bad immutable fact. An unexpected CMS failure remains 500 and
HAL retries the same fact ID/body. CMS debug diagnostics retain only safe error
type/class and PostgreSQL SQLSTATE where available; HAL records only fact ID,
fact type, HTTP status, and a bounded stable receiver error code. Neither side
logs a fact body, credential, token, or raw database error.

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

For an unmaterialized `ACCEPTED_FOR_DELIVERY`, `PROTOCOL_ACKNOWLEDGED`, or
older reconciliation-required start, CMS waits
`HAL_V1_START_RECONCILE_AFTER` and then queries HAL's exact
`GET /v1/transactions?cms_start_intent_id={uuid}` socket. A returned
authoritative transaction uses the same locked materializer as
`transaction.started`; it cannot create a second session. A 404 is not
transaction truth: CMS reconciles the exact command and makes the intent
explicitly reconciliation-required rather than claiming an active session.
Late start evidence after a CMS `REJECTED`/`EXPIRED` intent is retained through
the immutable fact receipt and moves the intent to reconciliation; it does not
silently fabricate a normal session or stop the physical HAL transaction.

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
reconciliation for pending/reconciliation-required mappings, stranded starts,
and exact command/transaction identity lookup. CPO charger creation, edits,
and status changes leave a
durable mapping record and attempt provider synchronization only after their
CMS transaction commits. Provider failure remains a visible pending state; it
never fabricates command/session outcome.

Full Postgres-to-HAL-to-virtual-charger acceptance has not yet been run because
the required disposable CMS/HAL databases and virtual charger topology were not
configured in this slice. Do not claim physical charger acceptance, restart
recovery, or full vertical-slice completion until the planned dual-service tests
prove them.

## Real-Hardware Mapping and Start Admission

Every CMS-to-HAL mapping/start/stop mutation uses a canonical nonzero UUID as
`X-Correlation-ID`. A reconciliation attempt creates a new UUID rather than
sending a worker label; the label is recorded separately as safe mapping
diagnostic context. CMS records a bounded category, provider HTTP status/code
when safe, operation, and correlation UUID on failed mapping delivery. It never
stores a provider response body, credential, or request payload.

CMS sends `chargers.serial_number` as optional `expected_serial` mapping
evidence. HAL still treats `charger_ocpp_identity` as canonical: both
`/{identity}` and `/{identity}/{serial}` are accepted only for a known enabled
mapping; a configured serial rejects a conflicting URL suffix, while an absent
suffix remains compatible. HAL stores Boot metadata as observed hardware
evidence only; it cannot overwrite CMS inventory.

Customer start admission remains independent from display occupancy. A fresh
online connector with raw OCPP `Available` or `Preparing` may accept a
CMS-controlled start. `Preparing` remains customer-visible as `CHARGING` and
all other, stale, offline, unknown, existing-intent, or occupying-session cases
remain blocked by the existing transaction/constraint path.
