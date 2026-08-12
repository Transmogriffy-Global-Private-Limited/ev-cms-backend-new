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
  session, and fleet state. It centralizes freshness: connector availability is
  never fresh when parent connection evidence is stale, offline, or unknown.
- Durable `operational_events` are written in the same transaction as an
  accepted fact projection. They are notification-sized; REST snapshots remain
  the authority after missed delivery or reconnect.

## Customer Flow

```text
customer bearer + CPO app ID
-> CMS locks wallet and resolves User Group > charger > hub tariff
-> CMS freezes tariff/GST, derives integer Wh affordability, creates hold,
   start intent, one-use appv1_ credential hash, and command identity
-> CMS synchronizes the exact charger/connector mapping then requests HAL start
-> HAL fact transaction.started materializes one CMS ACTIVE session
-> HAL meter/connection/status facts update CMS projections
-> customer stop persists one CMS stop command and asks HAL
-> HAL transaction.completed settles the wallet once from exact final Wh
```

The raw credential is sent to HAL only for the start command; CMS retains only
its SHA-256 value. Meter values remain integer Wh and are never interpolated.
The v1 tariff currently contains energy and GST components only; no fixed or
time component is invented by this implementation.

## Configuration and Security

`HAL_V1_BASE_URL`, `HAL_V1_CMS_BEARER_TOKEN`, and
`HAL_V1_CMS_FACT_BEARER_TOKEN` configure the two directions. When a base URL is
set both tokens are required; when no base URL is supplied customer charging
returns `hal_unavailable`. Local topology uses loopback endpoints only.

No token, raw credential, authorization header, or fact body is logged. The
HAL client does not retry a mutating command with a new identity. A caller
reconciles the same command identity after a transport ambiguity.

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
